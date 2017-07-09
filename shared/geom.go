package shared

import (
	"math/rand"

	"github.com/faiface/pixel"
)

func RandVec(min, max float64) pixel.Vec {
	return pixel.V((max-min)*(rand.Float64()-1/2), (max-min)*(rand.Float64()-1/2))
}

func RectFromCenter(center pixel.Vec, w, h float64) pixel.Rect {
	return pixel.R(center.X-w/2, center.Y-h/2, center.X+w/2, center.Y+h/2)
}
