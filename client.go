package rayon

import (
	"bytes"
	"context"
	"crypto/sha256"
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

type materialEntry struct {
	ref, sourceURL string
	scripts        []ScriptMaterial
	expires        time.Time
	size           int
	sequence       uint64
}
type materialFlight struct {
	done    chan struct{}
	success bool
}

// Client owns its HTTP connection pool and an in-memory bounded material cache.
// It is safe for concurrent calls. Close clears retained script source.
type Client struct {
	apiKey, baseURL string
	timeout         time.Duration
	http            *http.Client
	transport       *http.Transport
	mu              sync.Mutex
	closed          bool
	cache           map[[32]byte]*materialEntry
	flights         map[[32]byte]*materialFlight
	cacheBytes      int
	sequence        uint64
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
	c := &Client{apiKey: apiKey, baseURL: cfg.baseURL, timeout: cfg.timeout, transport: tr, cache: map[[32]byte]*materialEntry{}, flights: map[[32]byte]*materialFlight{}}
	c.http = &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}
func (c *Client) String() string   { return "Rayon Client(credentials=[REDACTED])" }
func (c *Client) GoString() string { return c.String() }
func (c *Client) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.cache = map[[32]byte]*materialEntry{}
		c.cacheBytes = 0
		for k, f := range c.flights {
			delete(c.flights, k)
			close(f.done)
		}
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

// Generate produces fresh header or form telemetry. The sole automatic retry is
// an explicitly uncharged HTTP 422 material_expired rejection with retained source.
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
	scripts := body.Scripts
	var key [32]byte
	haveKey := false
	if scripts != nil {
		b, _ := marshalJSON([]any{body.SourceURL, scripts})
		key = sha256.Sum256(b)
		haveKey = true
	} else {
		c.mu.Lock()
		for k, cached := range c.cache {
			if cached.ref == body.MaterialRef && cached.sourceURL == body.SourceURL {
				key = k
				scripts = append([]ScriptMaterial(nil), cached.scripts...)
				haveKey = true
				break
			}
		}
		c.mu.Unlock()
	}
	recovered := false
	for {
		if ctx.Err() != nil {
			return nil, contextError(ctx)
		}
		var flight *materialFlight
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, newError("validation")
		}
		if haveKey && scripts != nil {
			if pending := c.flights[key]; pending != nil {
				c.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, contextError(ctx)
				case <-pending.done:
					if !pending.success {
						return nil, newError("network")
					}
					continue
				}
			}
			cached := c.cache[key]
			if cached != nil && cached.expires.After(time.Now().Add(5*time.Second)) {
				body.Scripts = nil
				body.MaterialRef = cached.ref
			} else {
				body.MaterialRef = ""
				body.Scripts = scripts
				if len(c.flights) < 128 {
					flight = &materialFlight{done: make(chan struct{})}
					c.flights[key] = flight
				}
			}
		}
		c.mu.Unlock()
		v, meta, err := c.send(ctx, "POST", "/v1/shape", &body)
		var result *ShapeResult
		if err == nil {
			result, err = parseResult(v, meta, c.apiKey, body.Mode)
		}
		c.mu.Lock()
		if err == nil && haveKey && scripts != nil && body.Scripts != nil && !c.closed {
			c.remember(key, scripts, body.SourceURL, result)
		}
		if flight != nil && c.flights[key] == flight {
			delete(c.flights, key)
			flight.success = err == nil
			close(flight.done)
		}
		if err != nil {
			apiErr, ok := err.(*Error)
			if !recovered && scripts != nil && body.MaterialRef != "" && ok && apiErr.Kind == "http" && apiErr.StatusCode == 422 && apiErr.Code == "material_expired" && apiErr.BillingStatus == "not_charged" {
				if cached := c.cache[key]; cached != nil && cached.ref == body.MaterialRef {
					cached.expires = time.Time{}
				}
				recovered = true
				c.mu.Unlock()
				continue
			}
		}
		c.mu.Unlock()
		return result, err
	}
}

// remember is called with the cache mutex held; it performs no network work.
func (c *Client) remember(key [32]byte, scripts []ScriptMaterial, sourceURL string, r *ShapeResult) {
	b, _ := marshalJSON([]any{sourceURL, scripts})
	size := len(b)
	if size > 16*1024*1024 {
		return
	}
	if old := c.cache[key]; old != nil {
		c.cacheBytes -= old.size
		delete(c.cache, key)
	}
	for len(c.cache) > 0 && (len(c.cache) >= 128 || c.cacheBytes+size > 16*1024*1024) {
		var oldest [32]byte
		sequence := ^uint64(0)
		for k, v := range c.cache {
			if v.sequence < sequence {
				sequence = v.sequence
				oldest = k
			}
		}
		c.cacheBytes -= c.cache[oldest].size
		delete(c.cache, oldest)
	}
	expires, _ := time.Parse(time.RFC3339Nano, r.MaterialExpiresAt)
	c.sequence++
	c.cache[key] = &materialEntry{ref: r.MaterialRef, sourceURL: sourceURL, scripts: append([]ScriptMaterial(nil), scripts...), expires: expires, size: size, sequence: c.sequence}
	c.cacheBytes += size
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
func (c *Client) send(ctx context.Context, method, path string, body *ShapeRequest) (any, *Error, error) {
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
	if resp.StatusCode != 200 {
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
