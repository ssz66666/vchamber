package schedule

import (
	"errors"
	"sync"
)

// StorageBackendType is an int specifying the backend type
type StorageBackendType int

// ReadOnlyStorage is an interface defining a read-only map[string]string key-value store
type ReadOnlyStorage interface {
	BackendType() StorageBackendType
	Get(string) string
}

// Storage is an interface defining a read-write map[string]string key-value store
type Storage interface {
	BackendType() StorageBackendType
	Get(string) string
	Set(string, string)
	Del(string)
}

type memBackend struct {
	m     map[string]string
	mutex *sync.RWMutex
}

func (b *memBackend) Get(k string) string {
	b.mutex.RLock()
	v := b.m[k]
	b.mutex.RUnlock()
	return v
}

func (b *memBackend) Set(k string, v string) {
	b.mutex.Lock()
	b.m[k] = v
	b.mutex.Unlock()
}

func (b *memBackend) Del(k string) {
	b.mutex.Lock()
	delete(b.m, k)
	b.mutex.Unlock()
}

func (b *memBackend) BackendType() StorageBackendType {
	return StorageBackendMem
}

// StorageBackendType enum instances
const (
	StorageBackendMem StorageBackendType = iota
	StorageBackendRedis
)

// NewStorageBackend creates a new store with given type and initialisation arguments
func NewStorageBackend(typ StorageBackendType, args ...string) (Storage, error) {
	switch typ {
	case StorageBackendMem:
		return &memBackend{
			m:     make(map[string]string),
			mutex: &sync.RWMutex{},
		}, nil
	case StorageBackendRedis:
		return nil, errors.New("Redis backend not implemented")
	default:
		return nil, errors.New("Unsupported backend type")
	}
}
