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
	store, err := schedule.NewStorageBackend(schedule.StorageBackendRedis, schedule.RedisClientSentinel, *redis)
	if err != nil {
		log.Fatal(err)
	}

	rp := schedule.NewLoadBalancedReverseProxy(store)
	log.Fatal(http.ListenAndServe(*wsaddr, rp.GetProxy()))
}
