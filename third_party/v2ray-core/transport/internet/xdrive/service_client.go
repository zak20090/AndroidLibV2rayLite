package xdrive

import (
	"context"
	stdtls "crypto/tls"
	"net/http"
	"time"

	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
	coretls "github.com/v2fly/v2ray-core/v5/transport/internet/tls"
)

func newServiceClient(settings *internet.MemoryStreamConfig, timeout time.Duration, maxConns int) *http.Client {
	var socketSettings *internet.SocketConfig
	tlsConfig := &stdtls.Config{MinVersion: stdtls.VersionTLS12}
	if settings != nil {
		socketSettings = settings.SocketSettings
		if config := coretls.ConfigFromStreamSettings(settings); config != nil {
			tlsConfig = config.GetTLSConfig(coretls.WithNextProto("http/1.1"))
		}
	}
	if len(tlsConfig.NextProtos) == 0 {
		tlsConfig.NextProtos = []string{"http/1.1"}
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		IdleConnTimeout:     60 * time.Second,
		MaxIdleConns:        maxConns,
		MaxIdleConnsPerHost: maxConns,
		MaxConnsPerHost:     maxConns,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			dest, err := net.ParseDestination("tcp:" + addr)
			if err != nil {
				return nil, err
			}
			return internet.DialSystem(ctx, dest, socketSettings)
		},
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}
