package main

import (
	"net"

	"github.com/mmogo/mmo/shared"
)

type serverPlayer struct {
	player   *shared.Player
	conn     net.Conn
	requests chan *shared.Message
}

func newServerPlayer(player *shared.Player, conn net.Conn) *serverPlayer {
	return &serverPlayer{
		player:   player,
		conn:     conn,
		requests: make(chan *shared.Message),
	}
}
