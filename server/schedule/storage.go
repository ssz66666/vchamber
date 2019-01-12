package schedule

import (
	"errors"
	"sync"
	"time"

	"github.com/go-redis/redis"
)

const RedisEntryTTL = 5 * time.Minute
const (
	RedisClientNormal   = "normal"
	RedisClientSentinel = "sentinel"
)

// StorageBackendType is an int specifying the backend type
type StorageBackendType int

// ReadOnlyStorage is an interface defining a read-only map[string]string key-value store
type ReadOnlyStorage interface {
	BackendType() StorageBackendType
	Get(string) (string, error)
}

// Storage is an interface defining a read-write map[string]string key-value store
type Storage interface {
	BackendType() StorageBackendType
	Get(string) (string, error)
	Set(string, string) error
	Del(string) error
}

type memBackend struct {
	m     map[string]string
	mutex *sync.RWMutex
}

func (b *memBackend) Get(k string) (string, error) {
	b.mutex.RLock()
	v := b.m[k]
	b.mutex.RUnlock()
	return v, nil
}

func (b *memBackend) Set(k string, v string) error {
	b.mutex.Lock()
	b.m[k] = v
	b.mutex.Unlock()
	return nil
}

func (b *memBackend) Del(k string) error {
	b.mutex.Lock()
	delete(b.m, k)
	b.mutex.Unlock()
	return nil
}

func (b *memBackend) BackendType() StorageBackendType {
	return StorageBackendMem
}

type redisBackend struct {
	RedisClient *redis.Client
}

func (b *redisBackend) Get(k string) (string, error) {
	return b.RedisClient.Get(k).Result()
}

func (b *redisBackend) Set(k string, v string) error {
	return b.RedisClient.Set(k, v, RedisEntryTTL).Err()
}

func (b *redisBackend) Del(k string) error {
	return b.RedisClient.Del(k).Err()
}

func (b *redisBackend) BackendType() StorageBackendType {
	return StorageBackendRedis
}

// StorageBackendType enum instances
const (
	StorageBackendMem StorageBackendType = iota
	StorageBackendRedis
)

// NewRedisStorage wraps an existing redis connection to a storage
func NewRedisStorage(c *redis.Client) Storage {
	return &redisBackend{RedisClient: c}
}

// NewStorageBackend creates a new store with given type and initialisation arguments
func NewStorageBackend(typ StorageBackendType, args ...string) (Storage, error) {
	switch typ {
	case StorageBackendMem:
		return &memBackend{
			m:     make(map[string]string),
			mutex: &sync.RWMutex{},
		}, nil
	case StorageBackendRedis:
		if len(args) < 2 {
			return nil, errors.New("Redis backend requires at least two arguments")
		}
		var c *redis.Client
		if args[0] == RedisClientNormal {
			c = redis.NewClient(&redis.Options{
				Addr:     args[1],
				Password: "", // assume empty password
				DB:       0,  // default DB
			})

		} else if args[0] == RedisClientSentinel {
			c = redis.NewFailoverClient(&redis.FailoverOptions{
				MasterName:    "mymaster",
				SentinelAddrs: []string{args[1]},
			})
		} else {
			return nil, errors.New("Redis backend: invalid argument")
		}
		if err := c.Ping().Err(); err != nil {
			return nil, errors.New("Redis backend: failed to connect to redis master")
		}
		return &redisBackend{RedisClient: c}, nil
	default:
		return nil, errors.New("Unsupported backend type")
	}
}
