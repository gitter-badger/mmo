package shared

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/faiface/pixel"
)

const fatalErrSig = "**FATAL_ERR**"

type ClientPlayer struct {
	Player *Player
	Color  color.Color
}

type Player struct {
	// global unique UUID
	ID string
	// cartesian coordinates
	Position pixel.Vec
	// player speech; max buffer size 4
	SpeechBuffer []SpeechMesage
	// if set to false, player is treaded as though it has been deleted
	// this allows us to activate/deactivate players without deleting from state
	Active bool
}

type SpeechMesage struct {
	Txt       string
	Timestamp time.Time
}

type fatalError struct {
	err error
}

func FatalErr(err error) error {
	return &fatalError{err: err}
}

func (e *fatalError) Error() string {
	return fmt.Sprintf("%s: %v", fatalErrSig, e.err)
}

func IsFatal(err error) bool {
	return err != nil && strings.Contains(err.Error(), fatalErrSig)
}
