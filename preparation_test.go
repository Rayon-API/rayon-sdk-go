package rayon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const prepID = "prep_0123456789abcdef"

func prepFrame(state string) map[string]any {
	v := map[string]any{"preparation_id": prepID, "state": state, "status_url": "/v1/materials/preparations/" + prepID, "retry_after_ms": 0, "preparation_expires_at": "2099-01-01T00:00:00Z", "material_ref": "ref_0123456789abcdef", "material_expires_at": "2099-01-01T00:00:00Z", "ready_until": nil, "execution_backend": nil, "error": nil}
	if state == "failed" || state == "expired" {
		v["error"] = map[string]any{"code": "preparation_failed"}
	}
	if state == "ready" {
		v["ready_until"] = "2099-01-01T00:00:00Z"
		v["execution_backend"] = "rust"
	}
	return v
}
func TestPrepareExplicitFullContextAndUniqueKeys(t *testing.T) {
	var keys []string
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/materials/prepare" || r.Method != "POST" {
			t.Error("unexpected endpoint")
		}
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Error("auth")
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		var body ShapeRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.Scripts[0].Source != shape().Scripts[0].Source {
			t.Error("lost context")
		}
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(prepFrame("queued"))
	})
	for i := 0; i < 2; i++ {
		p, e := c.Prepare(context.Background(), shape())
		if e != nil || p.State != "queued" {
			t.Fatalf("%#v %v", p, e)
		}
	}
	if len(keys) != 2 || len(keys[0]) < 16 || keys[0] == keys[1] {
		t.Fatalf("invalid keys %v", keys)
	}
}
func TestPreparationStrictValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"state", func(v map[string]any) { v["state"] = "unknown" }},
		{"url", func(v map[string]any) { v["status_url"] = "https://evil.example/job" }},
		{"unknown", func(v map[string]any) { v["scripts"] = []any{} }},
		{"retry", func(v map[string]any) { v["retry_after_ms"] = 60001 }},
		{"backend", func(v map[string]any) { v["execution_backend"] = nil }},
		{"expiry", func(v map[string]any) { v["material_expires_at"] = "yesterday" }},
		{"id", func(v map[string]any) { v["preparation_id"] = "bad" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				v := prepFrame("ready")
				tc.change(v)
				json.NewEncoder(w).Encode(v)
			})
			_, e := c.GetPreparation(context.Background(), prepID)
			kind(t, e, "invalid_response")
		})
	}
	for _, raw := range []string{`{"state":"queued","state":"ready"}`, `{"state":NaN}`} {
		t.Run(raw, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, raw) })
			_, e := c.GetPreparation(context.Background(), prepID)
			kind(t, e, "invalid_response")
		})
	}
}
func TestPreparationWaitReadyFailedAndContextCancellation(t *testing.T) {
	state := "ready"
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.RequestURI(), "/v1/materials/preparations/"+prepID+"?wait=") {
			t.Error("poll path")
		}
		json.NewEncoder(w).Encode(prepFrame(state))
	})
	p, e := c.WaitUntilReady(context.Background(), prepID)
	if e != nil || p.ExecutionBackend != "rust" {
		t.Fatalf("%#v %v", p, e)
	}
	state = "failed"
	_, e = c.WaitUntilReady(context.Background(), prepID)
	if kind(t, e, "preparation_failed").PreparationID != prepID {
		t.Fatal("missing resume ID")
	}
	state = "queued"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e = c.WaitUntilReady(ctx, prepID)
	if kind(t, e, "timeout").PreparationID != prepID || time.Since(start) > time.Second {
		t.Fatal("unbounded wait")
	}
}
func TestPreparationRejectIDBeforeRequest(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("request sent") })
	_, e := c.GetPreparation(context.Background(), "../other")
	kind(t, e, "validation")
}

func TestPreparationStatesAndTerminalErrorSanitization(t *testing.T) {
	for _, state := range []string{"queued", "learning", "validating", "publishing", "ready", "failed", "expired"} {
		t.Run(state, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				v := prepFrame(state)
				if state == "failed" || state == "expired" {
					v["error"] = map[string]any{"code": "preparation_failed", "reason": "untrusted_reason", "action": "PRIVATE SOURCE", "retryable": false}
				}
				json.NewEncoder(w).Encode(v)
			})
			p, e := c.GetPreparation(context.Background(), prepID)
			if e != nil || p.State != state {
				t.Fatalf("%#v %v", p, e)
			}
			if p.Error != nil && (p.Error.Action != "" || p.Error.Reason != "") {
				t.Fatalf("unsafe error %#v", p.Error)
			}
		})
	}
}
func TestPreparationRejectsBadCalendarAndNullableFields(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["preparation_expires_at"] = "2099-02-30T00:00:00Z" },
		func(v map[string]any) { v["ready_until"] = "2099-01-01T00:00:00Z" },
		func(v map[string]any) { v["error"] = nil },
		func(v map[string]any) { v["error"] = map[string]any{"code": "failed", "reason": nil} },
		func(v map[string]any) { v["error"] = map[string]any{"code": "failed", "action": nil} },
		func(v map[string]any) { v["error"] = map[string]any{"code": "failed", "retryable": nil} },
	} {
		c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			v := prepFrame("failed")
			change(v)
			json.NewEncoder(w).Encode(v)
		})
		_, e := c.WaitUntilReady(context.Background(), prepID)
		if kind(t, e, "invalid_response").PreparationID != prepID {
			t.Fatal("missing resume ID")
		}
	}
}
func TestPreparationRetriesOnlyTransientGET(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"code":"unavailable"}}`)
			return
		}
		json.NewEncoder(w).Encode(prepFrame("ready"))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, e := c.WaitUntilReady(ctx, prepID)
	if e != nil || p.State != "ready" || calls != 2 {
		t.Fatalf("%#v %v %d", p, e, calls)
	}
	for _, status := range []int{400, 401, 403, 404, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"code":"denied"}}`)
			})
			_, e := c.WaitUntilReady(context.Background(), prepID)
			if e == nil || calls != 1 {
				t.Fatalf("%v %d", e, calls)
			}
			if typed, ok := e.(*Error); !ok || typed.PreparationID != prepID {
				t.Fatal("missing resume ID")
			}
		})
	}
}
func TestPreparationNeverRetriesAmbiguousPOSTAndRejectsGET202(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"code":"unavailable"}}`)
	})
	_, e := c.Prepare(context.Background(), shape())
	if e == nil || calls != 1 {
		t.Fatalf("%v %d", e, calls)
	}
	c = clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(prepFrame("queued"))
	})
	_, e = c.GetPreparation(context.Background(), prepID)
	kind(t, e, "http")
}
func TestPrepareAndWaitExplicitSequence(t *testing.T) {
	var paths []string
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		state := "ready"
		if r.Method == "POST" {
			state = "queued"
			w.WriteHeader(202)
		}
		json.NewEncoder(w).Encode(prepFrame(state))
	})
	r, e := c.PrepareAndWait(context.Background(), shape())
	if e != nil || r.State != "ready" || len(paths) != 2 || paths[0] != "/v1/materials/prepare" {
		t.Fatalf("%#v %v %v", r, e, paths)
	}
}
func TestPreparationIDsNeverRetainCredentials(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("credential ID request sent") })
	id := "prep_secret-key_0123456789"
	_, e := c.GetPreparation(context.Background(), id)
	er := kind(t, e, "validation")
	if er.PreparationID != "" || strings.Contains(fmt.Sprintf("%#v", er), "secret-key") {
		t.Fatal("retained credential")
	}
	_, e = c.WaitUntilReady(context.Background(), id)
	er = kind(t, e, "validation")
	if er.PreparationID != "" {
		t.Fatal("retained credential ID")
	}
}
func TestPreparationTerminalErrorPreservesSafeMetadata(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		v := prepFrame("failed")
		v["error"] = map[string]any{"code": "preparation_failed", "retryable": false}
		json.NewEncoder(w).Encode(v)
	})
	_, e := c.WaitUntilReady(context.Background(), prepID)
	er := kind(t, e, "preparation_failed")
	if er.Code != "preparation_failed" || er.Retryable == nil || *er.Retryable {
		t.Fatal("lost failure metadata")
	}
}
func TestPreparationErrorStringBounds(t *testing.T) {
	for _, field := range []string{"reason", "action"} {
		n := 129
		if field == "action" {
			n = 2049
		}
		c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			v := prepFrame("failed")
			v["error"] = map[string]any{"code": "preparation_failed", field: strings.Repeat("a", n)}
			json.NewEncoder(w).Encode(v)
		})
		_, e := c.GetPreparation(context.Background(), prepID)
		kind(t, e, "invalid_response")
	}
}
