package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

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
	c, e := vserver.Connect(nil, "ws://localhost:8080/ws", "testroom", "iamgod")
	if e != nil {
		fmt.Println(e)
		return
	}

	go c.ClientSendHeartbeat()
	c.ClientHandleRecv()
}
