package xdrive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

type Entry struct {
	Name   string
	Inline []byte
}

type Storage interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
	List(context.Context, string) ([]Entry, error)
	Close() error
}

var (
	storageMu sync.Mutex
	storages  = make(map[string]Storage)
)

func newStorage(settings *internet.MemoryStreamConfig) (Storage, error) {
	config, ok := settings.ProtocolSettings.(*Config)
	if !ok || config == nil {
		return nil, errInvalidConfig
	}
	if config.Service != "S3" {
		return nil, errUnsupportedService
	}
	keyParts := append([]string{config.RemoteFolder}, config.Secrets...)
	sum := sha256.Sum256([]byte(strings.Join(keyParts, "\x00")))
	key := hex.EncodeToString(sum[:])

	storageMu.Lock()
	defer storageMu.Unlock()
	if storage, ok := storages[key]; ok {
		return storage, nil
	}
	storage, err := newS3Storage(settings, config)
	if err != nil {
		return nil, err
	}
	storages[key] = storage
	return storage, nil
}
