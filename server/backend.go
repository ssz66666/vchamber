package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	vserver "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"
	"github.com/rs/cors"
)

var wsaddr = flag.String("ws", ":8080", "WebSocket Service bind address")
var restaddr = flag.String("rest", ":8081", "RESTful API bind address")

func main() {

	flag.Parse()

	server := vserver.NewServer()

	wsMux := vserver.NewVChamberWSMux(server)

	restMux := vserver.NewVChamberRestMux(server)

	go server.Run()
	server.AddRoom(vserver.NewRoom("testroom", server, "iamgod", "nobody"))

	go func() {
		withCORS := cors.Default().Handler(restMux)
		log.Fatal("RESTful API: ", http.ListenAndServe(*restaddr, withCORS))
	}()
	// TODO: use TLS
	go func() {
		err := http.ListenAndServe(*wsaddr, wsMux)
		if err != nil {
			log.Fatal("WSServer: ", err)
		}
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
