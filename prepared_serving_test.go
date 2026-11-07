package rayon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestPreparedServingRejectsSourcesBeforeNetwork(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	_, err := c.Generate(context.Background(), explicitShape())
	kind(t, err, "validation")
	if calls != 0 {
		t.Fatal("source upload reached generation")
	}
}
func TestPreparedServingMissNeverUploadsOrRetries(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var input map[string]any
		json.NewDecoder(r.Body).Decode(&input)
		if _, ok := input["scripts"]; ok {
			t.Error("generation uploaded sources")
		}
		if r.URL.Path != "/v1/shape" {
			t.Error("generation started preparation")
		}
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "material_expired", "billing_status": "not_charged"}})
	})
	_, err := c.Generate(context.Background(), shape())
	kind(t, err, "http")
	if calls != 1 {
		t.Fatalf("got %d calls", calls)
	}
}
func TestPreparedServingRejectsUnexpectedFallback(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "review-request-123")
		v := response("material_reference_1")
		v["path"] = "isolated"
		json.NewEncoder(w).Encode(v)
	})
	_, err := c.Generate(context.Background(), shape())
	kind(t, err, "invalid_response")
	if failure, ok := err.(*Error); !ok || failure.RequestID != "review-request-123" || failure.StatusCode != 200 {
		t.Fatal("protocol error lost sanitized response metadata")
	}
}
