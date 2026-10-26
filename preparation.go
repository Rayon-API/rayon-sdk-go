package rayon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Preparation is safe job metadata. It retains no source, cookies or telemetry.
type Preparation struct {
	PreparationID        string              `json:"preparation_id"`
	State                string              `json:"state"`
	StatusURL            string              `json:"status_url"`
	RetryAfterMS         int64               `json:"retry_after_ms"`
	PreparationExpiresAt string              `json:"preparation_expires_at"`
	MaterialRef          string              `json:"material_ref"`
	MaterialExpiresAt    string              `json:"material_expires_at"`
	ReadyUntil           *string             `json:"ready_until"`
	ExecutionBackend     *string             `json:"execution_backend"`
	Error                *PreparationFailure `json:"error"`
}

// ReadyMaterial contains metadata for a preparation confirmed ready by the server.
type ReadyMaterial struct {
	PreparationID        string `json:"preparation_id"`
	State                string `json:"state"`
	StatusURL            string `json:"status_url"`
	RetryAfterMS         int64  `json:"retry_after_ms"`
	PreparationExpiresAt string `json:"preparation_expires_at"`
	MaterialRef          string `json:"material_ref"`
	MaterialExpiresAt    string `json:"material_expires_at"`
	ReadyUntil           string `json:"ready_until"`
	ExecutionBackend     string `json:"execution_backend"`
}

// PreparationFailure contains sanitized metadata and trusted local guidance only.
type PreparationFailure struct {
	Code      string `json:"code"`
	Reason    string `json:"reason,omitempty"`
	Retryable *bool  `json:"retryable,omitempty"`
	Action    string `json:"action,omitempty"`
}

func prepError(kind, id string) *Error { e := newError(kind); e.PreparationID = id; return e }
func validPrepDate(v any) bool {
	s, ok := v.(string)
	if !ok || len(s) > 64 {
		return false
	}
	_, e := time.Parse(time.RFC3339Nano, s)
	return e == nil
}
func parsePreparation(v any, meta *Error, key, expected string) (*Preparation, error) {
	bad := func() (*Preparation, error) { meta.Kind = "invalid_response"; return nil, meta }
	m, ok := v.(map[string]any)
	if !ok || len(m) != 10 {
		return bad()
	}
	for _, name := range []string{"preparation_id", "state", "status_url", "retry_after_ms", "preparation_expires_at", "material_ref", "material_expires_at", "ready_until", "execution_backend", "error"} {
		if _, ok := m[name]; !ok {
			return bad()
		}
	}
	id := safe(text(m["preparation_id"]), key, refRE)
	if id == "" || (expected != "" && id != expected) || text(m["status_url"]) != "/v1/materials/preparations/"+id {
		return bad()
	}
	state := text(m["state"])
	switch state {
	case "queued", "learning", "validating", "publishing", "ready", "failed", "expired":
	default:
		return bad()
	}
	n, ok := m["retry_after_ms"].(json.Number)
	if !ok {
		return bad()
	}
	retry, e := n.Int64()
	if e != nil || retry < 0 || retry > 60000 {
		return bad()
	}
	ref := safe(text(m["material_ref"]), key, refRE)
	if ref == "" || !validPrepDate(m["material_expires_at"]) || !validPrepDate(m["preparation_expires_at"]) {
		return bad()
	}
	p := &Preparation{PreparationID: id, State: state, StatusURL: text(m["status_url"]), RetryAfterMS: retry,
		PreparationExpiresAt: text(m["preparation_expires_at"]), MaterialRef: ref, MaterialExpiresAt: text(m["material_expires_at"])}
	if state == "ready" {
		if text(m["execution_backend"]) != "rust" || !validPrepDate(m["ready_until"]) || m["error"] != nil {
			return bad()
		}
		until, backend := text(m["ready_until"]), "rust"
		p.ReadyUntil = &until
		p.ExecutionBackend = &backend
	} else if m["execution_backend"] != nil || m["ready_until"] != nil {
		return bad()
	}
	if (state == "failed" || state == "expired") != (m["error"] != nil) {
		return bad()
	}
	if m["error"] != nil {
		if state != "failed" && state != "expired" {
			return bad()
		}
		er, ok := m["error"].(map[string]any)
		if !ok {
			return bad()
		}
		for name := range er {
			if name != "code" && name != "reason" && name != "retryable" && name != "action" {
				return bad()
			}
		}
		code := safe(text(er["code"]), key, codeRE)
		if code == "" {
			return bad()
		}
		if x, ok := er["reason"]; ok {
			if reason, ok := x.(string); !ok || len(reason) > 128 {
				return bad()
			}
		}
		if x, ok := er["action"]; ok {
			if action, ok := x.(string); !ok || len(action) > 2048 {
				return bad()
			}
		}
		if x, ok := er["retryable"]; ok {
			if _, ok = x.(bool); !ok {
				return bad()
			}
		}
		safeMeta := metadata("http", 200, http.Header{}, map[string]any{"error": er}, key)
		p.Error = &PreparationFailure{Code: code, Reason: safeMeta.Reason, Retryable: safeMeta.Retryable, Action: safeMeta.Action}
	}
	return p, nil
}
func (p *Preparation) ready() (*ReadyMaterial, error) {
	if p.State == "failed" || p.State == "expired" {
		e := prepError("preparation_"+p.State, p.PreparationID)
		if p.Error != nil {
			e.Code = p.Error.Code
			e.Reason = p.Error.Reason
			e.Action = p.Error.Action
			e.Retryable = p.Error.Retryable
		}
		return nil, e
	}
	if p.State != "ready" {
		return nil, nil
	}
	return &ReadyMaterial{PreparationID: p.PreparationID, State: p.State, StatusURL: p.StatusURL, RetryAfterMS: p.RetryAfterMS,
		PreparationExpiresAt: p.PreparationExpiresAt, MaterialRef: p.MaterialRef, MaterialExpiresAt: p.MaterialExpiresAt,
		ReadyUntil: *p.ReadyUntil, ExecutionBackend: *p.ExecutionBackend}, nil
}

// Prepare explicitly creates a job; it never performs a billable generation.
// Each invocation receives a fresh idempotency key. POST is not automatically retried.
func (c *Client) Prepare(ctx context.Context, shape ShapeRequest) (*Preparation, error) {
	if ctx == nil || c.isClosed() {
		return nil, newError("validation")
	}
	body, e := validateShape(shape)
	if e != nil {
		return nil, e
	}
	var token [24]byte
	if _, e = rand.Read(token[:]); e != nil {
		return nil, newError("network")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	v, meta, e := c.send(ctx, "POST", "/v1/materials/prepare", &body, sendOptions{preparation: true, idempotency: hex.EncodeToString(token[:])})
	if e != nil {
		return nil, e
	}
	return parsePreparation(v, meta, c.apiKey, "")
}

// GetPreparation reads the current state without waiting or creating another job.
func (c *Client) GetPreparation(ctx context.Context, id string) (*Preparation, error) {
	if ctx == nil || !refRE.MatchString(id) || strings.Contains(id, c.apiKey) || c.isClosed() {
		return nil, newError("validation")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.getPreparation(ctx, id, 0)
}
func (c *Client) getPreparation(ctx context.Context, id string, wait float64) (*Preparation, error) {
	path := "/v1/materials/preparations/" + id + "?wait=" + strconv.Itoa(int(math.Floor(wait)))
	v, meta, e := c.send(ctx, "GET", path, nil, sendOptions{preparation: true})
	if e != nil {
		if typed, ok := e.(*Error); ok {
			typed.PreparationID = id
		}
		return nil, e
	}
	p, e := parsePreparation(v, meta, c.apiKey, id)
	if typed, ok := e.(*Error); ok {
		typed.PreparationID = id
	}
	return p, e
}
func transientPreparation(e error) bool {
	er, ok := e.(*Error)
	if !ok {
		return false
	}
	if er.Kind == "network" || er.Kind == "timeout" {
		return true
	}
	switch er.StatusCode {
	case 408, 429, 500, 502, 503, 504:
		return true
	}
	return false
}

// WaitUntilReady polls one existing job for at most 60 seconds and respects ctx.
// Cancelling the context cancels this wait, leaving the server job available to resume.
func (c *Client) WaitUntilReady(ctx context.Context, id string) (*ReadyMaterial, error) {
	if ctx == nil || !refRE.MatchString(id) || strings.Contains(id, c.apiKey) || c.isClosed() {
		return nil, newError("validation")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		if ctx.Err() != nil {
			e := contextError(ctx)
			e.PreparationID = id
			return nil, e
		}
		deadline, _ := ctx.Deadline()
		remaining := time.Until(deadline)
		requestCtx, requestCancel := context.WithTimeout(ctx, c.timeout)
		p, e := c.getPreparation(requestCtx, id, math.Min(10, math.Min(remaining.Seconds(), c.timeout.Seconds())))
		requestCancel()
		delay := 10 * time.Millisecond
		if e == nil {
			r, terminal := p.ready()
			if terminal != nil || r != nil {
				return r, terminal
			}
			delay = time.Duration(p.RetryAfterMS) * time.Millisecond
			if delay < 10*time.Millisecond {
				delay = 10 * time.Millisecond
			}
		} else {
			if ctx.Err() != nil {
				e := contextError(ctx)
				e.PreparationID = id
				return nil, e
			}
			if !transientPreparation(e) {
				return nil, e
			}
			jitter, _ := rand.Int(rand.Reader, big.NewInt(100))
			delay = 50 * time.Millisecond
			if jitter != nil {
				delay += time.Duration(jitter.Int64()) * time.Millisecond
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			e := contextError(ctx)
			e.PreparationID = id
			return nil, e
		case <-timer.C:
		}
	}
}

// PrepareAndWait explicitly creates a job, then waits until ready or terminal.
func (c *Client) PrepareAndWait(ctx context.Context, shape ShapeRequest) (*ReadyMaterial, error) {
	p, e := c.Prepare(ctx, shape)
	if e != nil {
		return nil, e
	}
	r, e := p.ready()
	if r != nil || e != nil {
		return r, e
	}
	return c.WaitUntilReady(ctx, p.PreparationID)
}
func (p Preparation) String() string {
	return fmt.Sprintf("RayonPreparation{ID:%q, State:%q}", p.PreparationID, p.State)
}
func (p Preparation) GoString() string { return p.String() }
func (p ReadyMaterial) String() string {
	return fmt.Sprintf("RayonReadyMaterial{ID:%q, State:%q}", p.PreparationID, p.State)
}
func (p ReadyMaterial) GoString() string { return p.String() }
