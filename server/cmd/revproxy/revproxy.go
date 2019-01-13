package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/UoB-Cloud-Computing-2018-KLS/vchamber/schedule"
)

var wsaddr = flag.String("ws", ":8080", "WebSocket Service bind address")
var redis = flag.String("redis", "redis-sentinel:26379", "Redis Sentinel address")

func main() {
	flag.Parse()

	// store, _ := schedule.NewStorageBackend(schedule.StorageBackendMem)
	// store.Set("testroom", "localhost:8080")

	store, err := schedule.NewStorageBackend(schedule.StorageBackendRedis, schedule.RedisClientSentinel, *redis)
	if err != nil {
		log.Fatal(err)
	}

	rp := schedule.NewLoadBalancedReverseProxy(store)
	mux := http.NewServeMux()
	mux.Handle("/ws", rp)
	// log.Fatal(http.ListenAndServe(*wsaddr, rp.GetProxy())
	log.Fatal(http.ListenAndServe(*wsaddr, mux))
}
