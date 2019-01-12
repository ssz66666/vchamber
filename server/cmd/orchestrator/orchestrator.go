package main

import (
	"flag"

	"github.com/UoB-Cloud-Computing-2018-KLS/vchamber/schedule"
	"github.com/go-redis/redis"
)

var restaddr = flag.String("addr", ":8080", "RESTful Service bind address")
var sentinel = flag.String("redis", "redis-sentinel:26379", "Redis Sentinel address")

func main() {
	flag.Parse()

	// store, _ := schedule.NewStorageBackend(schedule.StorageBackendMem)
	redisc := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    "mymaster",
		SentinelAddrs: []string{*sentinel},
	})
	store := schedule.NewRedisStorage(redisc)

	o := schedule.NewOrchestrator(redisc, store)

	o.Run()
}
