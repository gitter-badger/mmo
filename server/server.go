package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/faiface/pixel"
	"github.com/ilackarms/pkg/errors"
	"github.com/mmogo/mmo/shared"
	"github.com/soheilhy/cmux"
	"github.com/xtaci/smux"
)

type mmoServer struct {
	state *updateManager
}

func newMMOServer() *mmoServer {
	return &mmoServer{
		state: newUpdateManager(),
	}
}

func (s *mmoServer) start(protocol string, port int, errc chan error) error {
	laddr := fmt.Sprintf(":%v", port)
	//get client checksums
	clientChecksums := map[string]string{
		"client-windows-4.0-amd64.exe": "",
		"client-darwin-10.6-amd64":     "",
		"client-linux-amd64":           "",
	}
	//requires clients to be in same dir as server
	for client := range clientChecksums {
		f, err := os.Open(client)
		if err != nil {
			continue
		}
		h := md5.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		clientChecksums[client] = string(h.Sum(nil))
		log.Printf("serving client: %s", client)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for client, checksum := range clientChecksums {
			if strings.Contains(req.URL.Path, client) && req.URL.Query().Get("checksum") == checksum {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if strings.Contains(req.URL.Path, client) {
				log.Printf("serving client: %s", client)
				http.ServeFile(w, req, client)
				return
			}

		}
		log.Printf("bad http request: %s", req.URL.Path)
		http.NotFound(w, req)

	})

	l, err := shared.Listen(protocol, laddr)
	if err != nil {
		return fmt.Errorf("fatal: %v", err)
	}

	if protocol == shared.ProtocolTCP {
		// Create a cmux.
		m := cmux.New(l)
		httpL := m.Match(cmux.HTTP1Fast(), cmux.HTTP1())
		l = m.Match(cmux.Any())
		httpServer := &http.Server{
			Handler: mux,
		}
		go func() {
			go m.Serve()
			log.Printf("HTTP server crashed: %v", httpServer.Serve(httpL))
		}()
	} else {
		go func() {
			log.Printf("fileserver crashed: %v", http.ListenAndServe(laddr, mux))
		}()
	}

	// start game loop
	go s.gameLoop(errc)

	log.Printf("listening for connections on %v", port)
	for {
		conn, err := l.Accept()
		if err != nil {
			errc <- errors.New("failed to establish connection", err)
			continue
		}
		if err := s.handleConnection(conn); err != nil {
			errc <- errors.New("error handling connection", err)
			continue
		}
	}
}

func (s *mmoServer) handleConnection(conn net.Conn) error {
	session, err := smux.Server(conn, smux.DefaultConfig())
	if err != nil {
		return err
	}

	stream, err := session.AcceptStream()
	if err != nil {
		return err
	}

	conn = stream

	// read message
	msg, err := shared.GetMessage(conn, true)
	if err != nil {
		return err
	}

	// check first message is ConnectRequest
	if msg.Request == nil || msg.Request.ConnectRequest == nil {
		return errors.New("expected first message to be ConnectRequest", nil)
	}

	// get ID
	id := msg.Request.ConnectRequest.ID

	// add player
	if err := s.state.playerConnected(id, conn); err != nil {
		log.Printf("WARN: multiple login attempted from %s at %s\n", id, conn.RemoteAddr())
		return s.sendError(conn, shared.FatalErr(err))
	}

	// broadcast to clients that a new player has joined
	s.queueFunc(func() error {
		player, ok := s.state.world.GetPlayer(id)
		if !ok{
			return errors.New("player "+id+" no longer found", err)
		}
		return s.broadcastAddPlayer(id, player.Position)
	})

	// send world state to player
	s.queueFunc(func() error {
		return s.sendWorldState(id)
	})

	// handle player in goroutine
	go s.handlePlayer(id)

	log.Printf("new connected player %s from %s", id, conn.RemoteAddr().String())
	return nil
}

func (s *mmoServer) handlePlayer(id string) {
	for s.players[id] != nil {
		player := s.players[id]
		for len(player.RequestQueue) >= messagePerTickLimit {
			time.Sleep(time.Millisecond)
		}
		conn := player.Conn
		msg, err := shared.GetMessage(conn, true)
		if err != nil {
			log.Print(errors.New(fmt.Sprintf("Client disconnected: (failed getting message for player %s)", id), err))
			s.playersLock.Lock()
			delete(s.players, id)
			s.playersLock.Unlock()
			s.queueFunc(func() error {
				return s.broadcastPlayerDisconnected(id)
			})
			continue
		}
		log.Printf("%s %q", msg, id)
		if msg.Request != nil {
			player.QueueLock.Lock()
			player.RequestQueue = append(player.RequestQueue, msg)
			player.QueueLock.Unlock()
		}
	}
}

func (s *mmoServer) gameLoop(errc chan error) {
	last := time.Now()
	dt := 0.0
	for {
		dt += time.Since(last).Seconds()
		last = time.Now()
		if dt < tickTime {
			sleepTime := time.Duration(1000000*(tickTime-dt)) * time.Microsecond
			time.Sleep(sleepTime)
		}
		dt = 0.0
		if err := s.tick(); err != nil {
			log.Printf("ERROR IN TICK: %v", err)
			errc <- err
		}
	}
}

func (s *mmoServer) tick() error {
	for id, player := range s.players {
		player.QueueLock.Lock()
		for _, msg := range player.RequestQueue {
			if msg.Request != nil {
				switch {
				case msg.Request.MoveRequest != nil:
					s.handleMoveRequest(id, msg.Request.MoveRequest)
				case msg.Request.SpeakRequest != nil:
					s.handleSpeakRequest(id, msg.Request.SpeakRequest)
				}
			}
		}
		player.RequestQueue = []*shared.Message{}
		player.QueueLock.Unlock()
	}

	s.updatesLock.Lock()
	defer s.updatesLock.Unlock()
	processed := 0
	for _, update := range s.updates {
		if err := update(); err != nil {
			return errors.New("processing update", err)
		}
		processed++
	}
	s.updates = s.updates[processed:]
	return nil
}

func (s *mmoServer) handleMoveRequest(id string, req *shared.MoveRequest) error {
	s.playersLock.RLock()
	defer s.playersLock.RUnlock()
	player := s.players[id]
	if player == nil {
		return errors.New("requesting player "+id+" is nil??", nil)
	}

	player.Position = player.Position.Add(req.Direction.Scaled(2))
	s.queueFunc(func() error {
		return s.broadcastPlayerMoved(id, player.Position, req.Created)
	})
	return nil
}

func (s *mmoServer) handleSpeakRequest(id string, req *shared.SpeakRequest) error {
	s.playersLock.RLock()
	player := s.players[id]
	s.playersLock.RUnlock()
	if player == nil {
		return errors.New("requesting player "+id+" is nil??", nil)
	}
	s.queueFunc(func() error {
		return s.broadcastPlayerSpoke(id, req.Text)
	})
	return nil
}

func (s *mmoServer) queueFunc(fn func() error) {
	s.updatesLock.Lock()
	defer s.updatesLock.Unlock()
	s.updates = append(s.updates, fn)
}
