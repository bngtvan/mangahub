package main

import (
	"log"

	"mangahub/internal/tcp"
)

func main() {
	server := tcp.NewProgressSyncServer("9090")
	log.Fatal(server.Start())
}
