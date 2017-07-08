package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/pixelgl"
	"github.com/mmogo/mmo/shared"
)

type simulation struct {
	f       func()
	created time.Time
}

type GameWorld struct {
	playerID            string
	lock                sync.RWMutex
	players             map[string]*shared.ClientPlayer
	speechLock          sync.RWMutex
	playerSpeech        map[string][]string
	errc                chan error
	speechMode          bool
	currentSpeechBuffer string
	simulations         []*simulation
	runSimulations      []*simulation
	simLock             sync.Mutex
	wincenter           pixel.Vec
	centerMatrix        pixel.Matrix
	facing              shared.Direction
	action              shared.Action
}

func NewGame() *GameWorld {
	g := new(GameWorld)
	g.players = make(map[string]*shared.ClientPlayer)
	g.playerSpeech = make(map[string][]string)
	g.errc = make(chan error)
	return g
}

func (g *GameWorld) handleConnection(conn net.Conn) {
	loop := func() error {
		msg, err := shared.GetMessage(conn)
		if err != nil {
			return shared.FatalErr(err)
		}
		log.Println("RECV", msg)
		if msg.Error != nil {
			return fmt.Errorf("server returned an error: %v", msg.Error.Message)
		}
		if msg.Update != nil {
			g.ApplyUpdate(msg.Update)
		}
		return nil
	}
	for {
		if err := loop(); err != nil {
			g.errc <- err
			continue
		}

	}
}

func (g *GameWorld) ApplyUpdate(update *shared.Update) {
	if update == nil {
		log.Println("nil update")
		return
	}
	if update.PlayerMoved != nil {
		g.handlePlayerMoved(update.PlayerMoved)
	}
	if update.PlayerSpoke != nil {
		g.handlePlayerSpoke(update.PlayerSpoke)
	}
	if update.WorldState != nil {
		g.handleWorldState(update.WorldState)
	}
	if update.RemovePlayer != nil {
		g.handlePlayerDisconnected(update.RemovePlayer)
	}

}

func (g *GameWorld) handlePlayerMoved(moved *shared.PlayerMoved) {
	g.setPlayerPosition(moved.ID, moved.ToPosition)
	g.reapplySimulations(moved.RequestTime)
}

func (g *GameWorld) handlePlayerSpoke(speech *shared.PlayerSpoke) {
	id := speech.ID
	g.speechLock.Lock()
	defer g.speechLock.Unlock()
	txt, ok := g.playerSpeech[id]
	if !ok {
		txt = []string{}
	}
	if len(txt) > 4 {
		txt = txt[1:]
	}
	txt = append(txt, speech.Text)
	g.playerSpeech[id] = txt
	go func() {
		time.Sleep(time.Second * 5)
		g.speechLock.Lock()
		defer g.speechLock.Unlock()
		txt, ok := g.playerSpeech[id]
		if !ok {
			txt = []string{}
		}
		if len(txt) > 0 {
			txt = txt[1:]
		}
		g.playerSpeech[id] = txt
	}()
}

func (g *GameWorld) handleWorldState(worldState *shared.WorldState) {
	g.lock.Lock()
	defer g.lock.Unlock()
	for _, player := range worldState.Players {
		g.players[player.ID] = &shared.ClientPlayer{
			Player: player,
			Color:  stringToColor(player.ID),
		}
	}
}

func (g *GameWorld) handlePlayerDisconnected(disconnected *shared.RemovePlayer) {
	g.lock.Lock()
	defer g.lock.Unlock()
	delete(g.players, disconnected.ID)
}

func (g *GameWorld) processPlayerInput(conn net.Conn, win *pixelgl.Window) error {
	// set sprite facing if none
	if g.facing == shared.DIR_NONE {
		g.facing = shared.DOWN
	}
	g.action = shared.A_IDLE
	// mouse movement
	mousedir := shared.DIR_NONE
	if win.Pressed(pixelgl.MouseButtonLeft) {
		mouse := g.centerMatrix.Unproject(win.MousePosition())
		mousedir = shared.UnitToDirection(mouse.Unit())
		loc := g.players[g.playerID].Position
		g.queueSimulation(func() {
			g.setPlayerPosition(g.playerID, loc.Add(mouse.Unit().Scaled(2)))
		})

		// set sprite facing
		g.facing = mousedir
		g.action = shared.A_WALK

		// send to server
		if err := requestMove(mouse.Unit().Scaled(2), conn); err != nil {
			return err
		}
	}

	if g.speechMode {
		return g.processPlayerSpeechInput(conn, win)
	}
	if win.JustPressed(pixelgl.KeyEnter) {
		g.speechMode = true
		return nil
	}

	if mousedir != shared.DIR_NONE {
		return nil
	}

	return nil
}

func (g *GameWorld) processPlayerSpeechInput(conn net.Conn, win *pixelgl.Window) error {
	g.currentSpeechBuffer += win.Typed()
	if win.JustPressed(pixelgl.KeyBackspace) {
		if len(g.currentSpeechBuffer) < 1 {
			g.currentSpeechBuffer = ""
		} else {
			g.currentSpeechBuffer = g.currentSpeechBuffer[:len(g.currentSpeechBuffer)-1]
		}
	}
	if win.JustPressed(pixelgl.KeyEscape) {
		g.currentSpeechBuffer = ""
		g.speechMode = false
	}
	if win.JustPressed(pixelgl.KeyEnter) {
		var err error
		if len(g.currentSpeechBuffer) > 0 {
			err = requestSpeak(g.currentSpeechBuffer, conn)
		}
		g.currentSpeechBuffer = ""
		g.speechMode = false
		return err
	}
	return nil
}

func (g *GameWorld) setPlayerPosition(id string, pos pixel.Vec) {
	g.lock.RLock()
	defer g.lock.RUnlock()
	player, ok := g.players[id]
	if !ok {
		player = &shared.ClientPlayer{
			Player: &shared.Player{
				ID: id,
			},
			Color: stringToColor(id),
		}
		g.players[id] = player
	}
	player.Position = pos
}

func (g *GameWorld) queueSimulation(f func()) {
	g.simLock.Lock()
	g.simulations = append(g.simulations, &simulation{
		f:       f,
		created: time.Now(),
	})
	g.simLock.Unlock()
}

func (g *GameWorld) applySimulations() {
	g.simLock.Lock()
	for _, sim := range g.simulations {
		sim.f()
		g.runSimulations = append(g.runSimulations, sim)
	}
	g.simulations = []*simulation{}
	g.simLock.Unlock()
}

func (g *GameWorld) reapplySimulations(from time.Time) {
	i := 0
	if len(g.runSimulations) == 0 {
		return
	}
	g.simLock.Lock()
	for _, sim := range g.runSimulations {
		if sim.created.After(from) {
			break
		}
		i++
	}
	g.simulations = append(g.runSimulations[i:], g.simulations...)
	g.runSimulations = []*simulation{}
	g.simLock.Unlock()
}
