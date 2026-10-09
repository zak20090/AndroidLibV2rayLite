package xdrive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/errors"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

const (
	protocolName = "xdrive"
	sessionsDir  = "sessions"
	streamsDir   = "streams"
	uplinkDir    = "c2s"
	downlinkDir  = "s2c"
)

var errNotFound = errors.New("object not found")
var errInvalidConfig = errors.New("invalid xdrive settings")
var errUnsupportedService = errors.New("unsupported xdrive storage service")

func init() {
	common.Must(internet.RegisterProtocolConfigCreator(protocolName, func() interface{} {
		return new(Config)
	}))
	common.Must(internet.RegisterTransportDialer(protocolName, Dial))
}

func newSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("failed to generate session id").Base(err)
	}
	return hex.EncodeToString(buf), nil
}

func announceName(session string, at time.Time) string {
	return sessionsDir + "/" + strconv.FormatInt(at.UnixNano(), 10) + "-" + session
}

func streamConfig(streamSettings *internet.MemoryStreamConfig) (*Config, error) {
	config, ok := streamSettings.ProtocolSettings.(*Config)
	if !ok || config == nil {
		return nil, errors.New("invalid xdrive protocol settings")
	}
	return config, nil
}

func Dial(ctx context.Context, dest net.Destination, streamSettings *internet.MemoryStreamConfig) (internet.Connection, error) {
	config, err := streamConfig(streamSettings)
	if err != nil {
		return nil, err
	}
	storage, err := newStorage(streamSettings)
	if err != nil {
		return nil, err
	}
	session, err := newSessionID()
	if err != nil {
		return nil, err
	}
	if err := storage.Put(ctx, announceName(session, time.Now()), nil); err != nil {
		return nil, errors.New("failed to announce xdrive session").Base(err)
	}
	return newConn(ctx, storage, uplinkPrefix(session), downlinkPrefix(session), paramsFromConfig(config), nil), nil
}

func uplinkPrefix(session string) string {
	return streamsDir + "/" + session + "/" + uplinkDir
}

func downlinkPrefix(session string) string {
	return streamsDir + "/" + session + "/" + downlinkDir
}

func (c *Config) Validate() error {
	if c == nil || c.Service != "S3" || len(c.Secrets) != 1 || strings.Trim(c.RemoteFolder, "/") == "" {
		return errInvalidConfig
	}
	return validateS3Credentials(c)
}
