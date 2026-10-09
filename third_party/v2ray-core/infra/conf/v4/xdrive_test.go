package v4

import (
	"encoding/json"
	"testing"

	"github.com/v2fly/v2ray-core/v5/transport/internet"
	"github.com/v2fly/v2ray-core/v5/transport/internet/xdrive"
)

func TestXdriveStreamConfigBuild(t *testing.T) {
	var config StreamConfig
	err := json.Unmarshal([]byte(`{
		"network":"xdrive",
		"security":"none",
		"xdriveSettings":{
			"service":"S3",
			"remoteFolder":"private/device",
			"secrets":["{\"endpoint\":\"https://s3.example.test\",\"region\":\"ru-msk\",\"bucket\":\"test-bucket\",\"accessKey\":\"test\",\"secretKey\":\"test\"}"],
			"segmentBytes":262144,
			"concurrency":4
		}
	}`), &config)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := config.Build()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := internet.ToMemoryStreamConfig(stream)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := settings.ProtocolSettings.(*xdrive.Config)
	if !ok {
		t.Fatalf("transport settings type = %T, want *xdrive.Config", settings.ProtocolSettings)
	}
	if settings.ProtocolName != "xdrive" || got.Service != "S3" || got.RemoteFolder != "private/device" ||
		got.SegmentBytes != 262144 || got.Concurrency != 4 {
		t.Fatalf("unexpected xdrive transport settings: %#v / %#v", settings, got)
	}
}

func TestXdriveRequiresCredentials(t *testing.T) {
	var config StreamConfig
	if err := json.Unmarshal([]byte(`{"network":"xdrive","xdriveSettings":{"service":"S3","remoteFolder":"private/device"}}`), &config); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Build(); err == nil {
		t.Fatal("expected xdrive credential validation failure")
	}
}

func TestExistingTCPStreamConfigStillBuilds(t *testing.T) {
	var config StreamConfig
	if err := json.Unmarshal([]byte(`{"network":"tcp","tcpSettings":{"acceptProxyProtocol":true}}`), &config); err != nil {
		t.Fatal(err)
	}
	stream, err := config.Build()
	if err != nil {
		t.Fatal(err)
	}
	if stream.GetEffectiveProtocol() != "tcp" {
		t.Fatalf("protocol = %q, want tcp", stream.GetEffectiveProtocol())
	}
}
