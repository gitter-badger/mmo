package main

import (
	"log"
	"math"
	"time"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/imdraw"
	"golang.org/x/image/colornames"
)

func debugTiles() *pixel.Batch {
	start := time.Now()
	batch := pixel.NewBatch(&pixel.TrianglesData{}, nil)
	// http://flarerpg.org/tutorials/isometric_intro/
	batch.SetMatrix(pixel.IM.Rotated(pixel.ZV, 45*(math.Pi/180)).ScaledXY(pixel.ZV, pixel.V(1, 0.5)))
	imd := imdraw.New(nil)
	var i int
	for y := -10000.00; y <= 10000; y = y + 100 {
		for x := -10000.00; x <= 10000; x = x + 100 {
			i++
			imd.Color = colornames.Purple
			imd.Push(pixel.V(x, y))
			imd.Color = colornames.Green
			imd.Push(pixel.V(x+50, y+50))
			imd.Rectangle(0)
			imd.Color = colornames.Purple
			imd.Push(pixel.V(x+50, y+50))
			imd.Color = colornames.Green
			imd.Push(pixel.V(x+100, y+100))
		}
	}
	imd.Draw(batch)
	log.Printf("world render: %v iter took %s", i, time.Since(start))
	return batch
}
