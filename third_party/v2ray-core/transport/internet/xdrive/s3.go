// SPDX-License-Identifier: MPL-2.0
// Native private S3 backend for upstream XDRIVE. No public bucket policy,
// expiring presigned packet URLs, or HeadBucket permission is required.
package xdrive

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

const (
	s3BodyLimit = 20 << 20
	s3MaxEntries = 10000
	s3Attempts   = 4
)

// Secrets[0] is JSON to avoid adding fields to upstream's protobuf schema.
type s3Credentials struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}
type s3Storage struct {
	credentials s3Credentials
	endpoint    *url.URL
	prefix      string
	client      *http.Client
}

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var folderPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*/?$`)

func newS3Storage(settings *internet.MemoryStreamConfig, config *Config) (*s3Storage, error) {
	c, u, prefix, err := parseS3Credentials(config)
	if err != nil {
		return nil, err
	}
	client := newServiceClient(settings, 25*time.Second, 8)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &s3Storage{c, u, prefix, client}, nil
}

func validateS3Credentials(config *Config) error {
	_, _, _, err := parseS3Credentials(config)
	return err
}

func parseS3Credentials(config *Config) (s3Credentials, *url.URL, string, error) {
	if len(config.Secrets) != 1 {
		return s3Credentials{}, nil, "", errors.New("S3 requires one credentials JSON")
	}
	var c s3Credentials
	if json.Unmarshal([]byte(config.Secrets[0]), &c) != nil {
		return s3Credentials{}, nil, "", errors.New("invalid S3 credentials JSON")
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return s3Credentials{}, nil, "", errors.New("S3 endpoint must be an HTTPS origin without credentials, path or query")
	}
	if !bucketPattern.MatchString(c.Bucket) || c.Region == "" || c.AccessKey == "" || c.SecretKey == "" {
		return s3Credentials{}, nil, "", errors.New("S3 bucket, region and scoped credentials are required")
	}
	if !folderPattern.MatchString(config.RemoteFolder) {
		return s3Credentials{}, nil, "", errors.New("S3 requires a nonempty isolated prefix")
	}
	return c, u, strings.Trim(config.RemoteFolder, "/") + "/", nil
}

func safeS3Name(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func s3Escape(value string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' || c == '~' || keepSlash && c == '/' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func s3CanonicalQuery(query url.Values) string {
	pairs := make([]string, 0, len(query))
	for key, values := range query {
		for _, value := range values {
			pairs = append(pairs, s3Escape(key, false)+"="+s3Escape(value, false))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func s3HMACSHA256(key []byte, message string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return mac.Sum(nil)
}

func s3Authorize(method, host, path, query, payloadHash, accessKey, secretKey, region string, now time.Time) (string, string) {
	stamp := now.UTC().Format("20060102T150405Z")
	day := stamp[:8]
	const signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	headers := "host:" + host + "\nx-amz-content-sha256:" + payloadHash + "\nx-amz-date:" + stamp + "\n"
	canonicalRequest := strings.Join([]string{method, path, query, headers, signedHeaders, payloadHash}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	signingKey := s3HMACSHA256([]byte("AWS4"+secretKey), day)
	signingKey = s3HMACSHA256(signingKey, region)
	signingKey = s3HMACSHA256(signingKey, "s3")
	signingKey = s3HMACSHA256(signingKey, "aws4_request")
	signature := hex.EncodeToString(s3HMACSHA256(signingKey, stringToSign))
	authorization := "AWS4-HMAC-SHA256 Credential=" + accessKey + "/" + scope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
	return stamp, authorization
}

func (s *s3Storage) request(ctx context.Context, method, key string, query url.Values, data []byte) (int, []byte, error) {
	if len(data) > s3BodyLimit {
		return 0, nil, errors.New("S3 segment too large")
	}
	path := "/" + s.credentials.Bucket
	if key != "" {
		path += "/" + s3Escape(key, true)
	}
	canonicalQuery := s3CanonicalQuery(query)
	requestURL := s.endpoint.Scheme + "://" + s.endpoint.Host + path
	if canonicalQuery != "" {
		requestURL += "?" + canonicalQuery
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	for attempt := 0; attempt < s3Attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			case <-time.After(time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(data))
		if err != nil {
			return 0, nil, errors.New("invalid S3 request")
		}
		stamp, authorization := s3Authorize(method, req.URL.Host, path, canonicalQuery, hash,
			s.credentials.AccessKey, s.credentials.SecretKey, s.credentials.Region, time.Now())
		req.Header.Set("X-Amz-Date", stamp)
		req.Header.Set("X-Amz-Content-Sha256", hash)
		req.Header.Set("Authorization", authorization)
		resp, err := s.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return 0, nil, ctx.Err()
			}
			if attempt < s3Attempts-1 {
				continue
			}
			return 0, nil, errors.New("S3 network or certificate error")
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, s3BodyLimit+1))
		resp.Body.Close()
		if readErr != nil {
			return 0, nil, errors.New("S3 response read failed")
		}
		if len(body) > s3BodyLimit {
			return 0, nil, errors.New("S3 response exceeds safety limit")
		}
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < s3Attempts-1 {
			continue
		}
		return resp.StatusCode, body, nil
	}
	return 0, nil, errors.New("S3 retry budget exhausted")
}

func s3Status(operation string, status int) error {
	// Do not log response bodies: S3 errors can contain canonical requests/keys.
	return fmt.Errorf("S3 %s HTTP %d", operation, status)
}
func (s *s3Storage) Put(ctx context.Context, name string, data []byte) error {
	if !safeS3Name(name) {
		return errors.New("unsafe S3 object name")
	}
	status, _, err := s.request(ctx, "PUT", s.prefix+name, nil, data)
	if err != nil {
		return err
	}
	if status/100 != 2 {
		return s3Status("PUT", status)
	}
	return nil
}
func (s *s3Storage) Get(ctx context.Context, name string) ([]byte, error) {
	if !safeS3Name(name) {
		return nil, errors.New("unsafe S3 object name")
	}
	status, body, err := s.request(ctx, "GET", s.prefix+name, nil, nil)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, errNotFound
	}
	if status != 200 {
		return nil, s3Status("GET", status)
	}
	return body, nil
}

type s3Listing struct {
	Truncated bool   `xml:"IsTruncated"`
	Token     string `xml:"NextContinuationToken"`
	Contents  []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
	Common []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
}

func (s *s3Storage) listKeys(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	q := url.Values{"list-type": {"2"}, "prefix": {prefix}, "max-keys": {"1000"}}
	if !recursive {
		q.Set("delimiter", "/")
	}
	var keys []string
	seen := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		status, body, err := s.request(ctx, "GET", "", q, nil)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, s3Status("LIST", status)
		}
		var page s3Listing
		if xml.Unmarshal(body, &page) != nil {
			return nil, errors.New("invalid S3 listing XML")
		}
		add := func(key string) error {
			if !strings.HasPrefix(key, prefix) {
				return errors.New("S3 returned an object outside requested prefix")
			}
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
			if len(keys) > s3MaxEntries {
				return errors.New("S3 listing safety limit reached; clean stale sessions")
			}
			return nil
		}
		for _, v := range page.Contents {
			if err := add(v.Key); err != nil {
				return nil, err
			}
		}
		for _, v := range page.Common {
			if err := add(v.Prefix); err != nil {
				return nil, err
			}
		}
		if !page.Truncated {
			return keys, nil
		}
		if page.Token == "" || page.Token == q.Get("continuation-token") {
			return nil, errors.New("invalid S3 pagination token")
		}
		q.Set("continuation-token", page.Token)
	}
	return nil, errors.New("S3 pagination limit reached")
}
func (s *s3Storage) List(ctx context.Context, prefix string) ([]Entry, error) {
	if !safeS3Name(prefix) {
		return nil, errors.New("unsafe S3 prefix")
	}
	root := s.prefix + prefix + "/"
	keys, err := s.listKeys(ctx, root, false)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		name := strings.TrimSuffix(strings.TrimPrefix(key, root), "/")
		if name != "" && !strings.Contains(name, "/") {
			entries = append(entries, Entry{Name: name})
		}
	}
	return entries, nil
}
func (s *s3Storage) deleteKey(ctx context.Context, key string) error {
	status, _, err := s.request(ctx, "DELETE", key, nil, nil)
	if err != nil {
		return err
	}
	if status != 404 && status/100 != 2 {
		return s3Status("DELETE", status)
	}
	return nil
}
func (s *s3Storage) Delete(ctx context.Context, name string) error {
	if !safeS3Name(name) {
		return errors.New("unsafe S3 object name")
	}
	key := s.prefix + name
	if strings.HasPrefix(name, "streams/") && strings.Count(name, "/") == 1 {
		keys, err := s.listKeys(ctx, key+"/", true)
		if err != nil {
			return err
		}
		for _, k := range keys {
			if err := s.deleteKey(ctx, k); err != nil {
				return err
			}
		}
		return nil
	}
	return s.deleteKey(ctx, key)
}
func (s *s3Storage) Close() error {
	if transport, ok := s.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}
