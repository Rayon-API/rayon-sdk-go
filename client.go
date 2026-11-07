package rayon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Option configures a client. HTTP is allowed only on loopback development hosts.
type Option func(*clientConfig) error
type clientConfig struct {
	baseURL string
	timeout time.Duration
}

func WithBaseURL(base string) Option {
	return func(c *clientConfig) error {
		if !validURL(base) {
			return newError("validation")
		}
		u, e := url.Parse(base)
		if e != nil || u.RawQuery != "" || u.ForceQuery {
			return newError("validation")
		}
		if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
			return newError("validation")
		}
		c.baseURL = strings.TrimRight(base, "/")
		return nil
	}
}
func WithTimeout(timeout time.Duration) Option {
	return func(c *clientConfig) error {
		if timeout <= 0 || timeout > 2147483647*time.Millisecond {
			return newError("validation")
		}
		c.timeout = timeout
		return nil
	}
}

// Client owns its HTTP connection pool and is safe for concurrent calls.
// Solve requests never retain or upload script sources.
type Client struct {
	apiKey, baseURL string
	timeout         time.Duration
	http            *http.Client
	transport       *http.Transport
	mu              sync.Mutex
	closed          bool
}

func NewClient(apiKey string, options ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, newError("validation")
	}
	for _, c := range apiKey {
		if c < 33 || c > 126 {
			return nil, newError("validation")
		}
	}
	cfg := clientConfig{baseURL: "https://api.rayonapi.com", timeout: 45 * time.Second}
	for _, option := range options {
		if option == nil {
			return nil, newError("validation")
		}
		if e := option(&cfg); e != nil {
			return nil, newError("validation")
		}
	}
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ExpectContinueTimeout: time.Second, DisableCompression: true, MaxResponseHeaderBytes: 64 * 1024}
	c := &Client{apiKey: apiKey, baseURL: cfg.baseURL, timeout: cfg.timeout, transport: tr}
	c.http = &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}
func (c *Client) String() string   { return "Rayon Client(credentials=[REDACTED])" }
func (c *Client) GoString() string { return c.String() }
func (c *Client) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true

	}
	c.mu.Unlock()
	c.transport.CloseIdleConnections()
	return nil
}
func (c *Client) isClosed() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }
func contextError(ctx context.Context) *Error {
	if ctx.Err() == context.Canceled {
		return newError("aborted")
	}
	return newError("timeout")
}

// Generate uses ready managed material or a ready reference. It never prepares,
// uploads sources or retries automatically.
func (c *Client) Generate(ctx context.Context, shape ShapeRequest) (*ShapeResult, error) {
	if ctx == nil || c.isClosed() {
		return nil, newError("validation")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if ctx.Err() != nil {
		return nil, contextError(ctx)
	}
	body, e := validateShape(shape)
	if e != nil {
		return nil, e
	}
	v, meta, err := c.send(ctx, "POST", "/v1/shape", &body)
	if err != nil {
		return nil, err
	}
	frame, ok := v.(map[string]any)
	if !ok || (frame["path"] != nil && frame["path"] != "native") || frame["fallback_reason"] != nil {
		meta.Kind = "invalid_response"
		return nil, meta
	}
	return parseResult(v, meta, c.apiKey, body.Mode)
}

func (c *Client) Usage(ctx context.Context) (UsageResult, error) {
	if ctx == nil || c.isClosed() {
		return nil, newError("validation")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	v, meta, e := c.send(ctx, "GET", "/v1/usage", nil)
	if e != nil {
		return nil, e
	}
	m, ok := v.(map[string]any)
	if !ok {
		meta.Kind = "invalid_response"
		return nil, meta
	}
	return UsageResult(m), nil
}

type sendOptions struct {
	preparation bool
	idempotency string
}

func (c *Client) send(ctx context.Context, method, path string, body *ShapeRequest, opts ...sendOptions) (any, *Error, error) {
	var option sendOptions
	if len(opts) > 0 {
		option = opts[0]
	}
	if c.isClosed() {
		return nil, nil, newError("validation")
	}
	if ctx.Err() != nil {
		return nil, nil, contextError(ctx)
	}
	var rd io.Reader
	if body != nil {
		b, e := marshalJSON(body)
		if e != nil {
			return nil, nil, newError("validation")
		}
		rd = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if e != nil {
		return nil, nil, newError("validation")
	}
	r.Header.Set("Authorization", "Bearer "+c.apiKey)
	if option.idempotency != "" {
		r.Header.Set("Idempotency-Key", option.idempotency)
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Accept-Encoding", "identity")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, e := c.http.Do(r)
	if e != nil {
		if ctx.Err() != nil {
			return nil, nil, contextError(ctx)
		}
		return nil, nil, newError("network")
	}
	defer resp.Body.Close()
	kind := "http"
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		kind = "auth"
	} else if resp.StatusCode == 429 {
		kind = "rate_limit"
	}
	meta := metadata(kind, resp.StatusCode, resp.Header, nil, c.apiKey)
	var v any
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if readErr == nil && len(data) <= maxResponse && (resp.Header.Get("Content-Encoding") == "" || resp.Header.Get("Content-Encoding") == "identity") {
		v, readErr = strictJSON(data)
		if readErr == nil && body != nil && body.Mode == "form" && resp.StatusCode == 200 {
			m, ok := v.(map[string]any)
			if !ok || !formOrderMatches(data, m) {
				readErr = fmt.Errorf("invalid response")
			}
		}
	} else {
		readErr = fmt.Errorf("invalid response")
	}
	if ctx.Err() != nil {
		err := contextError(ctx)
		err.StatusCode = meta.StatusCode
		err.RequestID = meta.RequestID
		return nil, err, err
	}
	if resp.StatusCode != 200 && !(option.preparation && method == "POST" && resp.StatusCode == 202) {
		if readErr == nil {
			meta = metadata(kind, resp.StatusCode, resp.Header, v, c.apiKey)
		}
		return nil, meta, meta
	}
	if readErr != nil {
		meta.Kind = "invalid_response"
		return nil, meta, meta
	}
	return v, meta, nil
}
