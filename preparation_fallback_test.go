package rayon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestPreparationFallbackPreservesMetadataWithoutRetry(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Rayon-Capabilities") != "managed-fallback-v1" {
			t.Error("fallback capability missing")
		}
		out := response("ref_0123456789abcdef")
		out["path"] = "isolated"
		out["fallback_reason"] = "material_preparing"
		json.NewEncoder(w).Encode(out)
	})
	result, err := c.Generate(context.Background(), shape())
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "isolated" || result.FallbackReason != "material_preparing" {
		t.Fatal("fallback metadata lost")
	}
	if calls != 1 {
		t.Fatalf("generation attempted %d times", calls)
	}
}

func TestInvalidFallbackMetadataStaysInvalid(t *testing.T) {
	for _, metadata := range []map[string]any{
		{"path": "isolated"},
		{"path": "isolated", "fallback_reason": "other_reason"},
		{"path": "native", "fallback_reason": "material_preparing"},
		{"fallback_reason": "material_preparing"},
		{"path": "other", "fallback_reason": "material_preparing"},
	} {
		c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			out := response("ref_0123456789abcdef")
			for k, v := range metadata {
				out[k] = v
			}
			json.NewEncoder(w).Encode(out)
		})
		if _, err := c.Generate(context.Background(), shape()); err == nil {
			t.Fatal("inconsistent fallback accepted")
		}
	}
}
