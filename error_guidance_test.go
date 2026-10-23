package rayon

import (
	"net/http"
	"strings"
	"testing"
)

func TestIncompleteGenerationGivesNeutralGuidance(t *testing.T) {
	e := metadata("http", 422, http.Header{}, map[string]any{"error": map[string]any{
		"code": "generation_incomplete", "reason": "generation_incomplete", "billing_status": "pending", "retryable": false,
	}}, "synthetic-test-key")
	if e.Error() != "Generation did not complete." ||
		!strings.Contains(e.Action, "loader") ||
		!strings.Contains(e.Action, "If all required scripts were supplied") {
		t.Fatalf("missing actionable guidance: %s; %s", e.Error(), e.Action)
	}
	if e.Retryable == nil || *e.Retryable {
		t.Fatal("incomplete generation must not trigger automatic retries")
	}
}
