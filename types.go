// Package rayon provides a persistent, concurrency-safe client for Rayon Shape and usage.
package rayon

import (
	"encoding/json"
	"fmt"
)

// Profile contains only supplied browser fields. ValidateProfile checks the API contract.
type Profile map[string]any
type ScriptMaterial struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

// TargetRequest describes the exact target request. Nil pointers encode JSON null;
// pointers to empty strings preserve an explicitly empty body or content type.
type TargetRequest struct {
	URL         string  `json:"url"`
	Method      string  `json:"method"`
	ContentType *string `json:"content_type"`
	Body        *string `json:"body"`
}
type FormMetadata struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

// ShapeRequest selects managed material when Scripts and MaterialRef are omitted in header mode.
type ShapeRequest struct {
	SourceURL      string           `json:"source_url"`
	Scripts        []ScriptMaterial `json:"scripts,omitempty"`
	MaterialRef    string           `json:"material_ref,omitempty"`
	Profile        Profile          `json:"profile"`
	Request        TargetRequest    `json:"request"`
	DocumentCookie *string          `json:"document_cookie,omitempty"`
	Mode           string           `json:"mode,omitempty"`
	Form           *FormMetadata    `json:"form,omitempty"`
}

// UnmarshalJSON rejects unknown, duplicate and missing required fields, including
// missing body/content_type fields. Generate also validates constructed requests.
func (s *ShapeRequest) UnmarshalJSON(data []byte) error {
	v, err := strictJSON(data)
	if err != nil {
		return newError("validation")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return newError("validation")
	}
	for _, k := range []string{"source_url", "profile", "request"} {
		if _, ok := m[k]; !ok {
			return newError("validation")
		}
	}
	_, hasScripts := m["scripts"]
	_, hasRef := m["material_ref"]
	if hasScripts && hasRef {
		return newError("validation")
	}
	if hasScripts {
		if _, ok := m["scripts"].([]any); !ok {
			return newError("validation")
		}
	} else if hasRef {
		ref, ok := m["material_ref"].(string)
		if !ok || !refRE.MatchString(ref) {
			return newError("validation")
		}
	}
	for _, k := range []string{"document_cookie", "mode"} {
		if v, present := m[k]; present {
			if _, ok := v.(string); !ok {
				return newError("validation")
			}
		}
	}
	if v, present := m["form"]; present {
		form, ok := v.(map[string]any)
		if !ok || m["mode"] != "form" {
			return newError("validation")
		}
		for _, v := range form {
			if _, ok := v.(string); !ok {
				return newError("validation")
			}
		}
	}
	r, ok := m["request"].(map[string]any)
	if !ok || len(r) != 4 {
		return newError("validation")
	}
	for _, k := range []string{"url", "method", "content_type", "body"} {
		if _, ok := r[k]; !ok {
			return newError("validation")
		}
	}
	type plain ShapeRequest
	var p plain
	if err = decodeKnown(data, &p); err != nil {
		return newError("validation")
	}
	if _, err = validateShape(ShapeRequest(p)); err != nil {
		return err
	}
	*s = ShapeRequest(p)
	return nil
}

type ShapeResult struct {
	Path                string   `json:"path,omitempty"`
	MaterialReuse       *bool    `json:"material_reuse,omitempty"`
	DiscoveryDurationMS *float64 `json:"discovery_duration_ms,omitempty"`
	DiscoveryState      string   `json:"discovery_state,omitempty"`
	FallbackReason      string   `json:"fallback_reason,omitempty"`
	FallbackMessage     string   `json:"fallback_message,omitempty"`

	Headers           map[string]string `json:"headers,omitempty"`
	FormFields        map[string]string `json:"form_fields,omitempty"`
	FormBody          string            `json:"form_body,omitempty"`
	RequestID         string            `json:"request_id"`
	GenerationMS      int64             `json:"generation_ms"`
	ProfileUsed       Profile           `json:"profile_used"`
	MaterialRef       string            `json:"material_ref"`
	MaterialExpiresAt string            `json:"material_expires_at"`
}

func (r ShapeResult) String() string {
	return fmt.Sprintf("Rayon result(request_id=%s generation_ms=%d sensitive_fields=[REDACTED])", r.RequestID, r.GenerationMS)
}
func (r ShapeResult) GoString() string { return r.String() }

type UsageResult map[string]any

// String returns a pointer for optional request string fields.
func String(value string) *string { return &value }

// Compile-time check that ShapeRequest implements strict decoding.
var _ json.Unmarshaler = (*ShapeRequest)(nil)
