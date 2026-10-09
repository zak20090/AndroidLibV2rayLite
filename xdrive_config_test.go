package libv2ray

import (
	"strings"
	"testing"

	core "github.com/v2fly/v2ray-core/v5"
	coreserial "github.com/v2fly/v2ray-core/v5/infra/conf/serial"
)

func TestXdriveOutboundConfigLoads(t *testing.T) {
	configJSON := `{
		"outbounds":[{
			"protocol":"freedom",
			"tag":"direct",
			"settings":{},
			"streamSettings":{
				"network":"xdrive",
				"xdriveSettings":{
					"service":"S3",
					"remoteFolder":"private/device",
					"secrets":["{\"endpoint\":\"https://s3.example.test\",\"region\":\"ru-msk\",\"bucket\":\"test-bucket\",\"accessKey\":\"test\",\"secretKey\":\"test\"}"]
				}
			}
		}]
	}`
	config, err := coreserial.LoadJSONConfig(strings.NewReader(configJSON))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := core.New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExistingOutboundConfigStillLoads(t *testing.T) {
	configJSON := `{"outbounds":[{"protocol":"freedom","tag":"direct","settings":{},"streamSettings":{"network":"tcp"}}]}`
	if _, err := coreserial.LoadJSONConfig(strings.NewReader(configJSON)); err != nil {
		t.Fatal(err)
	}
}
