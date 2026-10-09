package xdrive

import (
	"context"

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

func newStorage(settings *internet.MemoryStreamConfig) (Storage, error) {
	if settings == nil {
		return nil, errInvalidConfig
	}
	config, ok := settings.ProtocolSettings.(*Config)
	if !ok || config == nil {
		return nil, errInvalidConfig
	}
	if config.Service != "S3" {
		return nil, errUnsupportedService
	}
	return newS3Storage(settings, config)
}
