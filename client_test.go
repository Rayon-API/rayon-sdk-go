package rayon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ptr(s string) *string { return &s }
func shape() ShapeRequest {
	return ShapeRequest{SourceURL: "https://example.com/page", Scripts: []ScriptMaterial{{URL: "https://example.com/protection.js", Source: "/* complete source */"}}, Profile: Profile{}, Request: TargetRequest{URL: "https://example.com/api", Method: "GET"}}
}
func response(ref string) map[string]any {
	return map[string]any{"headers": map[string]string{"X-Test-a": "a", "X-Test-b": "b", "X-Test-c": "c", "X-Test-d": "d", "X-Test-f": "f", "X-Test-z": "z", "X-Test-a0": "a0"}, "request_id": "req_1", "generation_ms": 2, "profile_used": map[string]any{}, "material_ref": ref, "material_expires_at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}
}
func clientFor(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, err := NewClient("secret-key", WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func kind(t *testing.T, err error, want string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Kind != want {
		t.Fatalf("wanted %s, got %#v", want, err)
	}
	return e
}
func TestHeaderUsageAndExactBody(t *testing.T) {
	var calls int
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Error("missing auth")
		}
		if r.URL.Path == "/v1/usage" {
			fmt.Fprint(w, `{"request_id":"usage_1","allowance":{"weekly_used":1},"wallet":{"balance_micros":1000000},"future":{"safe":true}}`)
			return
		}
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		tr := v["request"].(map[string]any)
		if tr["body"] != nil || tr["content_type"] != nil {
			t.Error("null fields missing")
		}
		json.NewEncoder(w).Encode(response("material_reference_1"))
	})
	r, err := c.Generate(context.Background(), shape())
	if err != nil || len(r.Headers) != 7 {
		t.Fatalf("%#v %v", r, err)
	}
	if _, err = c.Usage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestFormAndExactEmptyBody(t *testing.T) {
	s := shape()
	s.Mode = "form"
	s.Request.Method = "POST"
	s.Request.ContentType = ptr("application/x-www-form-urlencoded")
	s.Request.Body = ptr("")
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		var v ShapeRequest
		json.NewDecoder(r.Body).Decode(&v)
		if v.Request.Body == nil || *v.Request.Body != "" {
			t.Error("empty body lost")
		}
		vout := response("material_reference_1")
		delete(vout, "headers")
		vout["form_fields"] = map[string]string{"fooa": "a", "foob": "b", "fooc": "c", "food": "", "foof": "f", "fooz": "z"}
		vout["form_body"] = "fooa=a&foob=b&fooc=c&food=&foof=f&fooz=z"
		json.NewEncoder(w).Encode(vout)
	})
	r, err := c.Generate(context.Background(), s)
	if err != nil || len(r.FormFields) != 6 {
		t.Fatalf("%#v %v", r, err)
	}
}
func TestSafeErrorsNoRetriesOrRedirects(t *testing.T) {
	for _, status := range []int{401, 403, 429, 422, 500, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var n int
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				n++
				w.Header().Set("Location", "/elsewhere")
				w.Header().Set("X-Request-ID", "req_safe")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"code":"unknown_code","message":"secret-key raw traceback","reason":"secret-key","field":"__proto__.foo","domain":"https://secret-key.example","billing_status":"pending"}}`)
			})
			_, err := c.Generate(context.Background(), shape())
			k := "http"
			if status == 401 || status == 403 {
				k = "auth"
			}
			if status == 429 {
				k = "rate_limit"
			}
			e := kind(t, err, k)
			if n != 1 || strings.Contains(fmt.Sprintf("%+v", e), "secret-key") || e.Field != "" || e.Domain != "" || e.RequestID != "req_safe" {
				t.Fatalf("unsafe error or retry: %d %+v", n, e)
			}
		})
	}
}
func TestStrictInputAndResponse(t *testing.T) {
	for _, body := range []string{`{"allowance":{},"allowance":{}}`, `{"a":{"b":1,"b":2}}`, `[]`, `{"bad":"` + strings.Repeat("a", 256*1024) + `"}`, `{"a":NaN}`, `{"a":"` + string([]byte{0xff}) + `"}`} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			_, err := c.Usage(context.Background())
			kind(t, err, "invalid_response")
		})
	}
	var request ShapeRequest
	for _, raw := range []string{`{"source_url":"https://example.com","scripts":[],"profile":{},"request":{"url":"https://example.com","method":"GET"}}`, `{"unknown":true}`, `{"profile":{},"profile":{}}`} {
		if json.Unmarshal([]byte(raw), &request) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestCancellationTimeoutClose(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Generate(ctx, shape())
	kind(t, err, "aborted")
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = c.Generate(ctx, shape())
	kind(t, err, "timeout")
	c.Close()
	_, err = c.Usage(context.Background())
	kind(t, err, "validation")
}
func TestMaterialCoalescingAndRecovery(t *testing.T) {
	var uploads, refs atomic.Int32
	var expired atomic.Bool
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		var body ShapeRequest
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Scripts) > 0 {
			uploads.Add(1)
			time.Sleep(20 * time.Millisecond)
		} else {
			refs.Add(1)
			if expired.CompareAndSwap(true, false) {
				w.WriteHeader(422)
				fmt.Fprint(w, `{"error":{"code":"material_expired","billing_status":"not_charged"}}`)
				return
			}
		}
		json.NewEncoder(w).Encode(response("material_reference_1"))
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Generate(context.Background(), shape()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if uploads.Load() != 1 || refs.Load() != 19 {
		t.Fatalf("uploads %d refs %d", uploads.Load(), refs.Load())
	}
	expired.Store(true)
	if _, err := c.Generate(context.Background(), shape()); err != nil {
		t.Fatal(err)
	}
	if uploads.Load() != 2 || refs.Load() != 20 {
		t.Fatalf("recovery: %d %d", uploads.Load(), refs.Load())
	}
}
func TestProfileContract(t *testing.T) {
	var fixtures struct {
		Cases []struct {
			Name    string  `json:"name"`
			Profile Profile `json:"profile"`
			Valid   bool    `json:"valid"`
		} `json:"cases"`
	}
	data, err := osReadFixture()
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			expanded := expandFixture(tc.Profile)
			b, _ := json.Marshal(expanded)
			var profile Profile
			json.Unmarshal(b, &profile)
			_, err := ValidateProfile(profile)
			if (err == nil) != tc.Valid {
				t.Fatalf("valid=%v err=%v", tc.Valid, err)
			}
		})
	}
}

func TestRejectUnpairedSurrogates(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"x":"\ud800"}`) })
	_, err := c.Usage(context.Background())
	kind(t, err, "invalid_response")
}
func TestResponseFormOrder(t *testing.T) {
	s := shape()
	s.Mode = "form"
	s.Request.Method = "POST"
	s.Request.ContentType = ptr("application/x-www-form-urlencoded")
	s.Request.Body = ptr("")
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"request_id":"req_1","generation_ms":1,"profile_used":{},"material_ref":"material_reference_1","material_expires_at":%q,"form_fields":{"fooa":"a","foob":"b","fooc":"c","food":"","foof":"f","fooz":"z"},"form_body":"foob=b&fooa=a&fooc=c&food=&foof=f&fooz=z"}`, time.Now().Add(time.Minute).Format(time.RFC3339))
	})
	_, err := c.Generate(context.Background(), s)
	kind(t, err, "invalid_response")
}
func TestNoUnsafeMaterialRecovery(t *testing.T) {
	for _, billing := range []string{"pending", "unknown", ""} {
		t.Run(billing, func(t *testing.T) {
			var n int
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				n++
				if n == 1 {
					json.NewEncoder(w).Encode(response("material_reference_1"))
					return
				}
				w.WriteHeader(422)
				fmt.Fprintf(w, `{"error":{"code":"material_expired","billing_status":%q}}`, billing)
			})
			if _, e := c.Generate(context.Background(), shape()); e != nil {
				t.Fatal(e)
			}
			_, e := c.Generate(context.Background(), shape())
			kind(t, e, "http")
			if n != 2 {
				t.Fatalf("retried %d", n)
			}
		})
	}
}

func TestJSONPresenceAndNullRules(t *testing.T) {
	base := `{"source_url":"https://example.com/","scripts":[{"url":"https://example.com/a.js","source":"source"}],"profile":{},"request":{"url":"https://example.com/","method":"GET","content_type":null,"body":null}`
	for _, extra := range []string{`,"material_ref":null}`, `,"document_cookie":null}`, `,"mode":null}`, `,"form":null}`} {
		var s ShapeRequest
		if json.Unmarshal([]byte(base+extra), &s) == nil {
			t.Errorf("accepted %s", extra)
		}
	}
}
func TestCacheRefreshBoundsAndIsolation(t *testing.T) {
	var uploads atomic.Int32
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		var s ShapeRequest
		json.NewDecoder(r.Body).Decode(&s)
		if s.Scripts != nil {
			uploads.Add(1)
		}
		json.NewEncoder(w).Encode(response("material_reference_1"))
	})
	if _, e := c.Generate(context.Background(), shape()); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	for _, e := range c.cache {
		e.expires = time.Now().Add(4 * time.Second)
	}
	c.mu.Unlock()
	if _, e := c.Generate(context.Background(), shape()); e != nil {
		t.Fatal(e)
	}
	if uploads.Load() != 2 {
		t.Fatal("did not refresh")
	}
	for i := 0; i < 130; i++ {
		s := shape()
		s.Scripts[0].Source = fmt.Sprint(i)
		if _, e := c.Generate(context.Background(), s); e != nil {
			t.Fatal(e)
		}
	}
	c.mu.Lock()
	if len(c.cache) != 128 || c.cacheBytes > 16*1024*1024 {
		t.Errorf("cache bounds: %d %d", len(c.cache), c.cacheBytes)
	}
	c.mu.Unlock()
	c.Close()
	c.mu.Lock()
	if len(c.cache) != 0 || c.cacheBytes != 0 {
		t.Error("source retained on close")
	}
	c.mu.Unlock()
}

func TestRejectInvalidUTF8ProfileAndPort(t *testing.T) {
	for _, p := range []Profile{{"user_agent": string([]byte{0xff})}, {"languages": []string{string([]byte{0xff})}}, {"navigator": map[string]any{"vendor": string([]byte{0xff})}}} {
		if _, e := ValidateProfile(p); e == nil {
			t.Error("accepted invalid UTF8")
		}
	}
	if _, e := NewClient("key", WithBaseURL("http://127.0.0.1:99999")); e == nil {
		t.Error("accepted out of range port")
	}
	s := shape()
	s.Request.URL = "https://example.com:99999/path"
	if _, e := validateShape(s); e == nil {
		t.Error("accepted invalid target port")
	}
}

func TestQueuedCallerCancellationAndFailedUpload(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"private"}}`)
	})
	owner := make(chan error, 1)
	go func() { _, e := c.Generate(context.Background(), shape()); owner <- e }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() { _, e := c.Generate(ctx, shape()); queued <- e }()
	cancel()
	kind(t, <-queued, "aborted")
	if calls.Load() != 1 {
		t.Fatal("queued caller sent generation")
	}
	waiter := make(chan error, 1)
	go func() { _, e := c.Generate(context.Background(), shape()); waiter <- e }()
	time.Sleep(20 * time.Millisecond)
	close(release)
	kind(t, <-owner, "http")
	kind(t, <-waiter, "network")
	if calls.Load() != 1 {
		t.Fatal("failed owner allowed queued generation")
	}
}
func TestCacheBytesAndClientIsolation(t *testing.T) {
	var uploads atomic.Int32
	h := func(w http.ResponseWriter, r *http.Request) {
		var s ShapeRequest
		json.NewDecoder(r.Body).Decode(&s)
		if s.Scripts != nil {
			uploads.Add(1)
		}
		json.NewEncoder(w).Encode(response("material_reference_1"))
	}
	c := clientFor(t, h)
	other := clientFor(t, h)
	for _, client := range []*Client{c, other} {
		if _, e := client.Generate(context.Background(), shape()); e != nil {
			t.Fatal(e)
		}
	}
	if uploads.Load() != 2 {
		t.Fatal("clients shared cache")
	}
	for i := 0; i < 8; i++ {
		s := shape()
		s.Scripts = []ScriptMaterial{{URL: "https://example.com/a.js", Source: strings.Repeat("x", 2*1024*1024)}, {URL: "https://example.com/b.js", Source: strings.Repeat("y", 1024*1024-10) + fmt.Sprint(i)}}
		if _, e := c.Generate(context.Background(), s); e != nil {
			t.Fatal(e)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheBytes > 16*1024*1024 || len(c.cache) > 5 {
		t.Fatalf("byte cap failed: %d entries %d bytes", len(c.cache), c.cacheBytes)
	}
}
func TestTrustedGuidance(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"code":"generation_unavailable","reason":"generation_unavailable","billing_status":"unknown","retryable":true,"message":"secret-key private traceback","action":"leak"}}`)
	})
	_, e := c.Usage(context.Background())
	er := kind(t, e, "http")
	if er.Reason != "generation_unavailable" || !strings.Contains(er.Action, "Check request history") || strings.Contains(er.Error(), "secret-key") {
		t.Fatalf("unsafe guidance: %#v", er)
	}
}

func TestGenerationIntegerJSONNumber(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		v := response("material_reference_1")
		v["generation_ms"] = json.Number("2e0")
		json.NewEncoder(w).Encode(v)
	})
	r, e := c.Generate(context.Background(), shape())
	if e != nil || r.GenerationMS != 2 {
		t.Fatalf("%v %v", r, e)
	}
}
func TestInvalidProfilesAndRequests(t *testing.T) {
	for _, p := range []Profile{{"unknown": true}, {"screen": map[string]any{"width": 0}}, {"window": map[string]any{"devicePixelRatio": 17}}, {"user_agent": "first", "navigator": map[string]any{"userAgent": "different"}}, {"canvas": map[string]any{"data_url": "data:text/plain,a"}}, {"languages": []string{}}, {"navigator": map[string]any{"webdriver": "true"}}} {
		if _, e := ValidateProfile(p); e == nil {
			t.Errorf("accepted %#v", p)
		}
	}
	for _, change := range []func(*ShapeRequest){func(s *ShapeRequest) { s.Request.Body = ptr("") }, func(s *ShapeRequest) { s.DocumentCookie = ptr("a=b\r\n") }, func(s *ShapeRequest) { s.MaterialRef = "material_reference_1" }, func(s *ShapeRequest) { s.Mode = "form" }, func(s *ShapeRequest) { s.Form = &FormMetadata{} }} {
		s := shape()
		change(&s)
		if _, e := validateShape(s); e == nil {
			t.Errorf("accepted invalid shape")
		}
	}
}
