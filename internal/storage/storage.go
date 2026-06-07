// Package storage is a tiny, dependency-free client for S3-compatible object
// stores (AWS S3, Cloudflare R2, MinIO, DigitalOcean Spaces, …). It implements
// just what the dashboard needs — a single authenticated PUT — using AWS
// Signature Version 4, so no heavyweight SDK is pulled into the module.
//
// Configuration is entirely environment-driven (see internal/config). Public
// read access to uploaded objects is the deployer's responsibility (a public
// bucket, a bucket policy, or a CDN/custom domain via S3_PUBLIC_BASE_URL); set
// S3_OBJECT_ACL=public-read on providers that use object ACLs (e.g. AWS).
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Config configures an S3-compatible endpoint.
type Config struct {
	Endpoint       string // base endpoint, e.g. https://s3.eu-west-1.amazonaws.com
	Region         string // e.g. eu-west-1 (or "auto" for R2)
	Bucket         string
	AccessKey      string
	SecretKey      string
	PublicBaseURL  string // public base for serving objects (CDN/custom domain); optional
	ForcePathStyle bool   // path-style (host/bucket/key) — needed by MinIO and many others
	ACL            string // optional x-amz-acl, e.g. "public-read"
}

// Enabled reports whether enough is configured to talk to a bucket.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKey != "" && c.SecretKey != ""
}

// Store is a configured S3-compatible client.
type Store struct {
	cfg    Config
	http   *http.Client
	scheme string
	host   string // endpoint host[:port]
}

// New validates the config and returns a ready client.
func New(cfg Config) (*Store, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("storage: incomplete configuration")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return nil, fmt.Errorf("storage: invalid S3_ENDPOINT %q", cfg.Endpoint)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &Store{
		cfg:    cfg,
		http:   &http.Client{Timeout: 30 * time.Second},
		scheme: u.Scheme,
		host:   u.Host,
	}, nil
}

// target returns the request URL, the Host header, and the canonical URI (the
// signed path) for a key, honouring path-style vs virtual-host addressing.
func (s *Store) target(key string) (reqURL, host, canonURI string) {
	ek := encodeKey(key)
	if s.cfg.ForcePathStyle {
		host = s.host
		canonURI = "/" + awsURIEncode(s.cfg.Bucket) + "/" + ek
	} else {
		host = s.cfg.Bucket + "." + s.host
		canonURI = "/" + ek
	}
	return s.scheme + "://" + host + canonURI, host, canonURI
}

// PublicURL is where a stored object can be fetched from.
func (s *Store) PublicURL(key string) string {
	if s.cfg.PublicBaseURL != "" {
		return strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/" + encodeKey(key)
	}
	reqURL, _, _ := s.target(key)
	return reqURL
}

// Put uploads body under key with the given content type and returns its public
// URL. body is held in memory (callers cap the size), so SigV4 can hash it.
func (s *Store) Put(ctx context.Context, key, contentType string, body []byte) (string, error) {
	reqURL, host, canonURI := s.target(key)
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(body)

	// Headers to sign: lowercase names, trimmed values, sorted.
	headers := map[string]string{
		"host":                 host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	if contentType != "" {
		headers["content-type"] = contentType
	}
	if s.cfg.ACL != "" {
		headers["x-amz-acl"] = s.cfg.ACL
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(strings.TrimSpace(headers[k]))
		canonHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		canonURI,
		"", // no query string
		canonHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + s.cfg.Region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+s.cfg.SecretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.cfg.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	auth := "AWS4-HMAC-SHA256 Credential=" + s.cfg.AccessKey + "/" + scope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Host = host // Go sends this as the Host header; matches the signed "host"
	for k, v := range headers {
		if k == "host" {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", auth)
	req.ContentLength = int64(len(body))

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage: put: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("storage: put %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return s.PublicURL(key), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// encodeKey URI-encodes each path segment of an object key but keeps the
// segment separators ('/') literal.
func encodeKey(key string) string {
	parts := strings.Split(strings.TrimPrefix(key, "/"), "/")
	for i, p := range parts {
		parts[i] = awsURIEncode(p)
	}
	return strings.Join(parts, "/")
}

// awsURIEncode percent-encodes per RFC3986 / the SigV4 spec: only A-Z a-z 0-9
// and - _ . ~ are left unescaped.
func awsURIEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
