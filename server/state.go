package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/faiface/pixel"
	"github.com/mmogo/mmo/shared"
	"log"
)

var defaultStartingPosition = pixel.ZV

type localUpdate struct {
	update   *shared.Update
	onUpdate shared.UpdateCallback
}

type serverState struct {
	world                *shared.World
	connectedPlayers     map[string]*serverPlayer
	connectedPlayersLock sync.Mutex
	updates              chan *localUpdate
}

func newEmptyServerState() *serverState {
	return &serverState{
		world:            shared.NewEmptyWorld(),
		connectedPlayers: make(map[string]*serverPlayer),
		updates:          make(chan *localUpdate),
	}
}

// not threadsafe, only call from a single goroutine
func (state *serverState) step() error {
	select {
	default:
		return nil
	case localUpdate := <-state.updates:
		return state.world.ApplyUpdate(localUpdate.update, localUpdate.onUpdate)
	}
}

func (state *serverState) queueUpdate(update *shared.Update, onUpdate ...shared.UpdateCallback) {
	lu := &localUpdate{
		update: update,
	}
	if len(onUpdate) > 0 {
		lu.onUpdate = onUpdate[0]
	}
	go func() { state.updates <- lu }()
}

func (state *serverState) playerConnected(id string, conn net.Conn) error {
	if _, taken := state.connectedPlayers[id]; taken {
		return fmt.Errorf("Player ID %q in use", id)
	}
	state.connectedPlayersLock.Lock()
	state.connectedPlayers[id] = newServerPlayer(id, conn)
	state.connectedPlayersLock.Unlock()
	state.queueUpdate(&shared.Update{
		AddPlayer: &shared.AddPlayer{
			ID:       id,
			Position: defaultStartingPosition,
		},
	}, func(world *shared.World) {
		player, ok := world.CopyPlayer(id)
		if !ok {
			log.Printf("unexpected error: player %v not found after added to state", id)
		}
		
	})
	return nil
}

type serverPlayer struct {
	id       string
	conn     net.Conn
	requests chan *shared.Message
}

func newServerPlayer(id string, conn net.Conn) *serverPlayer {
	return &serverPlayer{
		id:       id,
		conn:     conn,
		requests: make(chan *shared.Message),
	}
}
