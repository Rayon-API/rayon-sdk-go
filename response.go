package rayon

import (
	"encoding/json"
	"math"
	"strings"
	"time"
)

func parseResult(v any, meta *Error, key, mode string) (*ShapeResult, error) {
	bad := func() (*ShapeResult, error) { meta.Kind = "invalid_response"; return nil, meta }
	m, ok := v.(map[string]any)
	if !ok {
		return bad()
	}
	id := safe(text(m["request_id"]), key, idRE)
	n, ok := m["generation_ms"].(json.Number)
	if !ok {
		return bad()
	}
	f, valid := number(n)
	if !valid || math.Trunc(f) != f || f < 0 || f > 9007199254740991 || id == "" {
		return bad()
	}
	ms := int64(f)
	profile, ok := m["profile_used"].(map[string]any)
	if !ok {
		return bad()
	}
	ref := text(m["material_ref"])
	expires := text(m["material_expires_at"])
	if !refRE.MatchString(ref) || len(expires) > 64 {
		return bad()
	}
	if _, e := time.Parse(time.RFC3339Nano, expires); e != nil {
		return bad()
	}
	r := &ShapeResult{RequestID: id, GenerationMS: ms, ProfileUsed: Profile(profile), MaterialRef: ref, MaterialExpiresAt: expires}
	if mode == "form" {
		if _, ok = m["headers"]; ok {
			return bad()
		}
		fields, ok := m["form_fields"].(map[string]any)
		if !ok || len(fields) != 6 {
			return bad()
		}
		r.FormFields = map[string]string{}
		family := ""
		slots := map[string]bool{}
		for name, v := range fields {
			s, ok := v.(string)
			match := formNameRE.FindStringSubmatch(name)
			if !ok || match == nil || len(s) > 16384 || controls(s) || (match[2] != "d" && s == "") {
				return bad()
			}
			if family != "" && family != match[1] {
				return bad()
			}
			family = match[1]
			slots[match[2]] = true
			r.FormFields[name] = s
		}
		if len(slots) != 6 {
			return bad()
		}
		body, ok := m["form_body"].(string)
		if !ok {
			return bad()
		}
		pairs, e := formPairs(body)
		if e != nil || len(pairs) != 6 {
			return bad()
		}
		seen := map[string]bool{}
		encoded := []string{}
		for _, p := range pairs {
			v, ok := r.FormFields[p[0]]
			if !ok || v != p[1] || seen[p[0]] {
				return bad()
			}
			seen[p[0]] = true
			encoded = append(encoded, formEscape(p[0])+"="+formEscape(p[1]))
		}
		if strings.Join(encoded, "&") != body {
			return bad()
		}
		r.FormBody = body
	} else {
		if _, ok = m["form_fields"]; ok {
			return bad()
		}
		if _, ok = m["form_body"]; ok {
			return bad()
		}
		headers, ok := m["headers"].(map[string]any)
		if !ok || (len(headers) != 6 && len(headers) != 7) {
			return bad()
		}
		r.Headers = map[string]string{}
		family := ""
		slots := map[string]bool{}
		for name, v := range headers {
			s, ok := v.(string)
			match := headerNameRE.FindStringSubmatch(name)
			if !ok || match == nil {
				return bad()
			}
			for _, c := range s {
				if c < 33 || c > 126 {
					return bad()
				}
			}
			f, suffix := strings.ToLower(match[1]), strings.ToLower(match[2])
			if family != "" && family != f || slots[suffix] || (suffix != "d" && strings.TrimSpace(s) == "") {
				return bad()
			}
			family = f
			slots[suffix] = true
			r.Headers[name] = s
		}
		for _, suffix := range []string{"a", "b", "c", "d", "f", "z"} {
			if !slots[suffix] {
				return bad()
			}
		}
	}
	return r, nil
}
