package rayon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestManagedMaterialOmissionAndMetadata(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if calls > 0 {
			if _, ok := body["scripts"]; ok {
				t.Error("automatic upload")
			}
			if _, ok := body["material_ref"]; ok {
				t.Error("automatic reference pinned")
			}
		}
		calls++
		out := response("ref_0123456789abcdef")
		out["path"] = "isolated"
		out["material_reuse"] = false
		out["discovery_duration_ms"] = 12.5
		out["discovery_state"] = "failed"
		out["fallback_reason"] = "unknown_material"
		out["fallback_message"] = "A fallback was used. Contact support with this request ID."
		json.NewEncoder(w).Encode(out)
	})
	if _, e := c.Generate(context.Background(), shape()); e != nil {
		t.Fatal(e)
	}
	s := shape()
	s.Scripts = nil
	for i := 0; i < 3; i++ {
		r, e := c.Generate(context.Background(), s)
		if e != nil {
			t.Fatal(e)
		}
		if r.Path != "isolated" || r.FallbackReason != "unknown_material" || r.MaterialReuse == nil || *r.MaterialReuse || r.DiscoveryDurationMS == nil || *r.DiscoveryDurationMS != 12.5 {
			t.Fatal("operation metadata missing")
		}
	}
	if _, e := c.Prepare(context.Background(), s); e == nil {
		t.Fatal("materialless preparation accepted")
	}
}
func TestManagedMaterialJSONBoundaries(t *testing.T) {
	base := `"source_url":"https://shop.example.com/","profile":{},"request":{"url":"https://shop.example.com/action","method":"GET","content_type":null,"body":null}`
	var s ShapeRequest
	if e := json.Unmarshal([]byte(`{`+base+`}`), &s); e != nil {
		t.Fatal(e)
	}
	for _, extra := range []string{`"scripts":null`, `"scripts":[]`, `"material_ref":null`, `"material_ref":""`, `"scripts":[],"material_ref":"ref_0123456789abcdef"`, `"mode":"form"`} {
		if e := json.Unmarshal([]byte(`{`+base+`,`+extra+`}`), &s); e == nil {
			t.Errorf("accepted %s", extra)
		}
	}
}

func TestInvalidOperationMetadata(t *testing.T) {
	for _, extra := range []map[string]any{{"path": "other"}, {"material_reuse": 1}, {"discovery_duration_ms": -1}, {"discovery_state": "unsafe detail"}, {"fallback_reason": "unsafe detail"}, {"fallback_message": "unsafe\nmessage"}} {
		c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			out := response("ref_0123456789abcdef")
			for k, v := range extra {
				out[k] = v
			}
			json.NewEncoder(w).Encode(out)
		})
		s := shape()
		s.Scripts = nil
		if _, e := c.Generate(context.Background(), s); e == nil {
			t.Fatal("accepted invalid operation metadata")
		}
	}
}
