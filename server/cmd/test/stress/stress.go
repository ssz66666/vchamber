package main

import (
	"flag"
	"fmt"
	"log"

	vsv "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"
)

var addr = flag.String("addr", "localhost:8080", "server to stress")
var rest = flag.String("restaddr", "localhost:8081", "RESTful API address for the server")
var nPerRoom = flag.Int("nc", 100, "number of clients per room")
var defaultRoomID = flag.String("rid", "testroom", "the roomID")
var defaultToken = flag.String("token", "iamgod", "the room token")

func main() {

	flag.Parse()
	var clients []*vsv.Client

	for i := 0; i < (*nPerRoom - 1); i++ {
		c, e := vsv.Connect(nil, "ws://"+*addr+"/ws", *defaultRoomID, *defaultToken)
		if e != nil {
			log.Printf("something wrong at n=%d", i)
			log.Fatal(e)
		}
		log.Printf("successfully joined %d clients", *nPerRoom)
		go c.ClientSendHeartbeat()
		clients = append(clients, c)
	}
	c, e := vsv.Connect(nil, "ws://"+*addr+"/ws", *defaultRoomID, *defaultToken)
	if e != nil {
		fmt.Println(e)
		return
	}

	c.ClientSendHeartbeat()
}
