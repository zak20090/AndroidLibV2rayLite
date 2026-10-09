# AndroidLibV2rayLite

## Build requirements
* JDK
* Android SDK
* Go
* gomobile

## Build instructions
1. `git clone [repo] && cd AndroidLibV2rayLite`
2. `gomobile init`
3. `go mod tidy -v`
4. `gomobile bind -v -androidapi 21 -ldflags='-s -w' ./`

## XDRIVE S3 transport

This library includes the **client/outbound side** of the XDRIVE transport for use
with an XDRIVE-capable server. The SigV4 S3 backend is adapted from
[RX-PRO-S3-Manager](https://github.com/cwash797-cmd/RX-PRO-S3-Manager); configure
XDRIVE as a transport on an existing outbound (for example VLESS), not as an
outbound protocol:

```json
{
  "protocol": "vless",
  "settings": {
    "vnext": [{
      "address": "server.example.com",
      "port": 443,
      "users": [{
        "id": "00000000-0000-0000-0000-000000000000",
        "encryption": "none"
      }]
    }]
  },
  "streamSettings": {
    "network": "xdrive",
    "security": "none",
    "xdriveSettings": {
      "service": "S3",
      "remoteFolder": "private/device-id",
      "secrets": [
        "{\"endpoint\":\"https://s3.example.com\",\"region\":\"region-1\",\"bucket\":\"private-bucket\",\"accessKey\":\"ACCESS_KEY\",\"secretKey\":\"SECRET_KEY\"}"
      ]
    }
  }
}
```

`remoteFolder` must be a dedicated object prefix. `secrets` must contain one JSON
string with an HTTPS endpoint origin, region, bucket, and scoped S3 access-key pair.
Treat the configuration as secret material: it contains credentials. S3 requests use
SigV4, do not follow redirects, and use the core's system dialer/socket settings.
Optional segment, polling, session-TTL, and concurrency settings use the XDRIVE
defaults when omitted.

The local V2Fly core under `third_party/v2ray-core` is based on v5.51.2 and contains
the parser and dialer integration required for `network: "xdrive"`; do not remove
the `replace` directive in `go.mod` unless the updated core is incorporated
elsewhere.
