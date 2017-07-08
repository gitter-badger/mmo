package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/faiface/pixel"
	"github.com/ilackarms/pkg/errors"
	"github.com/mmogo/mmo/shared"
)

var defaultStartingPosition = pixel.ZV

// update manager handles all updates
// update manager makes sure that updates are duplicated properly
// for the server's internal state, and broadcast to clients
// who are expected to apply updates to their internal state
type updateManager struct {
	world                *shared.World
	connectedPlayers     map[string]*serverPlayer
	connectedPlayersLock sync.RWMutex
	m                    *messenger
}

func newUpdateManager() *updateManager {
	return &updateManager{
		world:            shared.NewEmptyWorld(),
		connectedPlayers: make(map[string]*serverPlayer),
		m:                &messenger{},
	}
}

/*
	specific event handlers
*/
func (mgr *updateManager) playerConnected(id string, conn net.Conn) error {
	if _, taken := mgr.connectedPlayers[id]; taken {
		return fmt.Errorf("Player ID %q in use", id)
	}
	update := &shared.Update{
		AddPlayer: &shared.AddPlayer{
			ID:       id,
			Position: defaultStartingPosition,
		},
	}
	if err := mgr.world.ApplyUpdate(update); err != nil {
		return fmt.Errorf("failed to apply update %v: %v", update, err)
	}

	player, ok := mgr.world.GetPlayer(id)
	if !ok {
		return fmt.Errorf("player %s should have been added to state but was not", id)
	}

	mgr.connectedPlayersLock.Lock()
	mgr.connectedPlayers[id] = newServerPlayer(player, conn)
	mgr.connectedPlayersLock.Unlock()

	if err := mgr.broadcast(&shared.Message{Update: update}); err != nil {
		return errors.New("failed to broadcast update", err)
	}

	return nil
}

func (mgr *updateManager) sendError(conn net.Conn, err error) error {
	if err == nil {
		return errors.New("cannot send nil error!", nil)
	}
	return shared.SendMessage(&shared.Message{
		Error: &shared.Error{Message: err.Error()}}, conn)
}

func (mgr *updateManager) broadcast(msg *shared.Message) error {
	log.Println(msg)
	data, err := shared.Encode(msg)
	if err != nil {
		return err
	}
	mgr.connectedPlayersLock.RLock()
	defer mgr.connectedPlayersLock.RUnlock()
	for _, player := range mgr.connectedPlayers {
		log.Printf("sending update: %s to %s", msg, player.player.ID)
		player.conn.SetDeadline(time.Now().Add(time.Second))
		if err := shared.SendRaw(data, player.conn); err != nil {
			return err
		}
	}
	return nil
}
