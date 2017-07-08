package shared

import (
	"sync"
	"time"

	"github.com/ilackarms/pkg/errors"
)

type World struct {
	players     map[string]*Player
	playersLock sync.RWMutex
}

func NewEmptyWorld() *World {
	return &World{
		players: make(map[string]*Player),
	}
}

// WARNING: do not modify world in callback. it is intended for reads only
func (w *World) ApplyUpdate(update *Update) (err error) {
	if update.AddPlayer != nil {
		return w.addPlayer(update.AddPlayer)
	}
	if update.PlayerMoved != nil {
		return w.applyPlayerMoved(update.PlayerMoved)
	}
	if update.PlayerSpoke != nil {
		return w.applyPlayerSpoke(update.PlayerSpoke)
	}
	if update.WorldState != nil {
		return w.setWorldState(update.WorldState)
	}
	if update.RemovePlayer != nil {
		return w.applyRemovePlayer(update.RemovePlayer)
	}
	return errors.New("empty update given? wtf", nil)
}

// GetPlayer returns a referece to player
// PLEASE do not use this reference to modify player directly!
// Objects returned by GetPlayer should be read-only
// Looking forward to go supporting immutable references
func (w *World) GetPlayer(id string) (*Player, bool) {
	player, err := w.getPlayer(id)
	if err != nil {
		return nil, false
	}
	return player, true
}

// if player doesnt exist, add. if player is inactive, activate. if player is active, error
func (w *World) addPlayer(added *AddPlayer) error {
	if player, err := w.getPlayer(added.ID); err == nil {
		if player.Active {
			return errors.New("player "+added.ID+" already active!", nil)
		}
		player.Active = true
	}
	w.setPlayer(added.ID, &Player{
		ID:           added.ID,
		Position:     added.Position,
		SpeechBuffer: []SpeechMesage{},
		Active:       true,
	})
	return nil
}

func (w *World) applyPlayerMoved(moved *PlayerMoved) error {
	player, err := w.getActivePlayer(moved.ID)
	if err != nil {
		return err
	}
	player.Position = moved.ToPosition
	return nil
}

func (w *World) applyPlayerSpoke(speech *PlayerSpoke) error {
	id := speech.ID
	player, err := w.getActivePlayer(id)
	if err != nil {
		return err
	}
	txt := player.SpeechBuffer
	// speech  buffer size 4
	if len(txt) > 4 {
		txt = txt[1:]
	}
	txt = append(txt, SpeechMesage{Txt: speech.Text, Timestamp: time.Now()})
	w.setPlayer(id, player)
	return nil
}

func (w *World) setWorldState(worldState *WorldState) error {
	w = worldState
	return nil
}

func (w *World) applyRemovePlayer(removed *RemovePlayer) error {
	player, err := w.getActivePlayer(removed.ID)
	if err != nil {
		return err
	}
	player.Active = false
	return nil
}

func (w *World) getActivePlayer(id string) (*Player, error) {
	player, err := w.getPlayer(id)
	if err != nil {
		return nil, err
	}
	if !player.Active {
		return nil, errors.New("player "+id+" requested but inactive", nil)
	}
	return player, nil
}

func (w *World) getPlayer(id string) (*Player, error) {
	w.playersLock.RLock()
	player, ok := w.players[id]
	w.playersLock.RUnlock()
	if !ok {
		return nil, errors.New("player "+id+" requested but not found", nil)
	}
	return player, nil
}

func (w *World) setPlayer(id string, player *Player) {
	w.playersLock.Lock()
	w.players[id] = player
	w.playersLock.Unlock()
}
