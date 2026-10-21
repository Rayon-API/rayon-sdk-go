package rayon

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Error contains only sanitized metadata and trusted local guidance. Raw server
// messages and transport causes are deliberately excluded.
type Error struct {
	Kind                                                                      string
	StatusCode                                                                int
	RequestID, RetryAfter, Code, Reason, Action, Domain, Field, BillingStatus string
	Retryable                                                                 *bool
	message                                                                   string
}

func (e *Error) Error() string {
	if e.message != "" {
		return e.message
	}
	return messages[e.Kind]
}
func (e *Error) String() string { return e.Error() }
func (e *Error) GoString() string {
	return fmt.Sprintf("RayonError{Kind:%q, StatusCode:%d, RequestID:%q, Code:%q}", e.Kind, e.StatusCode, e.RequestID, e.Code)
}

var messages = map[string]string{"validation": "Invalid Rayon API request options.", "auth": "Rayon API denied authentication or access.", "rate_limit": "Rayon API request limit reached.", "http": "Rayon API returned an unsuccessful HTTP response.", "invalid_response": "Rayon API returned an invalid response.", "network": "Rayon API request could not be completed.", "timeout": "Rayon API request timed out.", "aborted": "Rayon API request was aborted."}

func newError(kind string) *Error { return &Error{Kind: kind} }

//go:embed error_catalog.json
var catalogJSON []byte

type guidance struct {
	Codes     []string `json:"codes"`
	Message   string   `json:"message"`
	Action    string   `json:"action"`
	Retryable bool     `json:"retryable"`
}

var catalog = func() map[string]guidance {
	var c map[string]guidance
	if json.Unmarshal(catalogJSON, &c) != nil {
		panic("invalid embedded Rayon error catalog")
	}
	return c
}()
var idRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var refRE = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
var codeRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var domainRE = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var fieldRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*$`)
var retryRE = regexp.MustCompile(`^[0-9]{1,10}$`)

func safe(s, key string, re *regexp.Regexp) string {
	if s != "" && !strings.Contains(s, key) && re.MatchString(s) {
		return s
	}
	return ""
}
func text(v any) string { s, _ := v.(string); return s }
func metadata(kind string, status int, h http.Header, v any, key string) *Error {
	e := newError(kind)
	e.StatusCode = status
	e.RequestID = safe(h.Get("X-Request-ID"), key, idRE)
	retry := h.Get("Retry-After")
	if !strings.Contains(retry, key) {
		if retryRE.MatchString(retry) {
			e.RetryAfter = retry
		} else if d, err := http.ParseTime(retry); err == nil && d.Format(http.TimeFormat) == retry {
			e.RetryAfter = retry
		}
	}
	m, _ := v.(map[string]any)
	er, _ := m["error"].(map[string]any)
	e.Code = safe(text(er["code"]), key, codeRE)
	if id := safe(text(er["request_id"]), key, idRE); id != "" {
		e.RequestID = id
	}
	e.Domain = safe(text(er["domain"]), key, domainRE)
	if len(e.Domain) > 253 {
		e.Domain = ""
	}
	e.Field = safe(text(er["field"]), key, fieldRE)
	if len(e.Field) > 96 {
		e.Field = ""
	}
	for _, part := range strings.Split(e.Field, ".") {
		if part == "__proto__" || part == "constructor" || part == "prototype" {
			e.Field = ""
		}
	}
	switch b := text(er["billing_status"]); b {
	case "not_charged", "pending", "unknown":
		e.BillingStatus = b
	}
	if b, ok := er["retryable"].(bool); ok {
		e.Retryable = &b
	}
	reason := text(er["reason"])
	if g, ok := catalog[reason]; ok && !strings.Contains(reason, key) {
		for _, code := range g.Codes {
			if code == e.Code {
				e.Reason = reason
				domain := e.Domain
				if domain == "" {
					domain = "this domain"
				}
				e.message = strings.ReplaceAll(g.Message, "{domain}", domain)
				e.Action = g.Action
				if g.Retryable && (e.BillingStatus != "not_charged" || (e.Retryable != nil && !*e.Retryable)) {
					e.Action = "Check request history before sending another request. Contact support with the request ID if it remains unclear."
				}
				break
			}
		}
	}
	return e
}
