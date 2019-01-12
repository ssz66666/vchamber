package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	vserver "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"
	"github.com/rs/cors"
)

var listenaddr = flag.String("addr", ":8080", "WebSocket Service bind address")

func main() {

	flag.Parse()

	server := vserver.NewServer()

	mux := vserver.NewVChamberRestMux(server)
	mux.HandleFunc("/ws", vserver.GetVChamberWSHandleFunc(server))

	go server.Run()
	server.AddRoom(vserver.NewRoom("testroom", server, "iamgod", "nobody"))

	go func() {
		withCORS := cors.Default().Handler(mux)
		log.Fatal("vChamber backend: ", http.ListenAndServe(*listenaddr, withCORS))
	}()

	// start a zombie client
	var c *vserver.Client
	var e error
	for {
		c, e = vserver.Connect(nil, "ws://localhost:8080/ws", "testroom", "iamgod")
		if e == nil {
			break
		} else {
			log.Println("failed to connect to local testroom, trying again!")
			time.Sleep(1 * time.Second)
		}
	}

	go c.ClientSendHeartbeat()
	c.ClientHandleRecv()
}
