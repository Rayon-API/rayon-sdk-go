package rayon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxResponse = 256 * 1024

func decodeKnown(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// Token parsing preserves JSON's object semantics without accepting duplicate keys.
func strictJSON(b []byte) (any, error) {
	if !utf8.Valid(b) || !validSurrogates(b) {
		return nil, errors.New("invalid JSON")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, e := readJSON(d, 0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("invalid JSON")
	}
	return v, nil
}
func readJSON(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("invalid JSON")
	}
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch t {
	case json.Delim('{'):
		m := map[string]any{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return nil, e
			}
			k, ok := key.(string)
			if !ok {
				return nil, errors.New("invalid JSON")
			}
			if _, ok = m[k]; ok {
				return nil, errors.New("duplicate JSON key")
			}
			v, e := readJSON(d, depth+1)
			if e != nil {
				return nil, e
			}
			m[k] = v
		}
		_, e = d.Token()
		return m, e
	case json.Delim('['):
		a := []any{}
		for d.More() {
			v, e := readJSON(d, depth+1)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		_, e = d.Token()
		return a, e
	}
	return t, nil
}

func validSurrogates(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		n, e := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if e != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			low, e := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if e != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

// formOrderMatches compares the telemetry wire body with JSON object insertion order.
func formOrderMatches(data []byte, m map[string]any) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw["form_fields"]))
	if t, e := d.Token(); e != nil || t != json.Delim('{') {
		return false
	}
	parts := []string{}
	for d.More() {
		t, e := d.Token()
		if e != nil {
			return false
		}
		name, ok := t.(string)
		if !ok {
			return false
		}
		var value string
		if d.Decode(&value) != nil {
			return false
		}
		parts = append(parts, formEscape(name)+"="+formEscape(value))
	}
	return strings.Join(parts, "&") == text(m["form_body"])
}
