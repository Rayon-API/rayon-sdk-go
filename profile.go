package rayon

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

//go:embed profile_defaults.json
var defaultsJSON []byte
var pngRE = regexp.MustCompile(`^data:image/png;base64,[A-Za-z0-9+/]+={0,2}$`)
var profileFields = map[string]string{"user_agent": "string", "platform": "string", "language": "string", "languages": "strings", "screen": "object", "window": "object", "webgl": "object", "canvas": "object", "navigator": "object"}
var nestedFields = map[string]map[string]string{
	"screen": {"width": "int", "height": "int", "availWidth": "int", "availHeight": "int", "colorDepth": "int", "pixelDepth": "int"},
	"window": {"innerWidth": "int", "innerHeight": "int", "outerWidth": "int", "outerHeight": "int", "screenX": "int", "screenY": "int", "screenLeft": "int", "screenTop": "int", "pageXOffset": "int", "pageYOffset": "int", "devicePixelRatio": "number"},
	"webgl":  {"renderer": "string", "vendor": "string"}, "canvas": {"data_url": "string"},
	"navigator": {"userAgent": "string", "platform": "string", "vendor": "string", "appVersion": "string", "appCodeName": "string", "appName": "string", "product": "string", "language": "string", "languages": "strings", "hardwareConcurrency": "int", "maxTouchPoints": "int", "webdriver": "bool", "cookieEnabled": "bool", "doNotTrack": "nullable"},
}

func number(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, e := n.Float64()
	return f, e == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
}
func checkProfileFields(m map[string]any, fields map[string]string) bool {
	for k, v := range m {
		typ, ok := fields[k]
		if !ok {
			return false
		}
		switch typ {
		case "string", "nullable":
			if typ == "nullable" && v == nil {
				continue
			}
			s, ok := v.(string)
			if !ok || (!(k == "data_url") && utf8.RuneCountInString(s) > 8192) {
				return false
			}
		case "strings":
			a, ok := v.([]any)
			if !ok || len(a) < 1 || len(a) > 32 {
				return false
			}
			for _, s := range a {
				v, ok := s.(string)
				if !ok || utf8.RuneCountInString(v) > 8192 {
					return false
				}
			}
		case "int", "number":
			f, ok := number(v)
			if !ok || (typ == "int" && (math.Trunc(f) != f || math.Abs(f) > 9007199254740991)) {
				return false
			}
		case "bool":
			if _, ok := v.(bool); !ok {
				return false
			}
		case "object":
			if _, ok := v.(map[string]any); !ok {
				return false
			}
		}
	}
	return true
}

// ValidateProfile validates and deep-copies supplied browser fields without adding defaults.
func ValidateProfile(p Profile) (Profile, error) {
	if !validProfileUTF8(reflect.ValueOf(p), 0) {
		return nil, newError("validation")
	}
	b, e := marshalJSON(p)
	if e != nil {
		return nil, newError("validation")
	}
	v, e := strictJSON(b)
	if e != nil {
		return nil, newError("validation")
	}
	m, ok := v.(map[string]any)
	if !ok || !checkProfileFields(m, profileFields) {
		return nil, newError("validation")
	}
	for section, fields := range nestedFields {
		if v, exists := m[section]; exists {
			if !checkProfileFields(v.(map[string]any), fields) {
				return nil, newError("validation")
			}
		}
	}
	nav, _ := m["navigator"].(map[string]any)
	for top, alias := range map[string]string{"user_agent": "userAgent", "platform": "platform", "language": "language", "languages": "languages"} {
		a, aok := m[top]
		b, bok := nav[alias]
		if aok && bok && !reflect.DeepEqual(a, b) {
			return nil, newError("validation")
		}
	}
	if canvas, ok := m["canvas"].(map[string]any); ok {
		if data, ok := canvas["data_url"]; ok && !pngRE.MatchString(text(data)) {
			return nil, newError("validation")
		}
	}
	ranges := map[string]map[string][2]float64{"screen": {"width": {1, 32768}, "height": {1, 32768}, "availWidth": {0, 32768}, "availHeight": {0, 32768}, "colorDepth": {1, 64}, "pixelDepth": {1, 64}}, "window": {"innerWidth": {1, 32768}, "innerHeight": {1, 32768}, "outerWidth": {1, 32768}, "outerHeight": {1, 32768}, "devicePixelRatio": {.1, 16}, "screenX": {-2147483648, 2147483647}, "screenY": {-2147483648, 2147483647}, "screenLeft": {-2147483648, 2147483647}, "screenTop": {-2147483648, 2147483647}, "pageXOffset": {-2147483648, 2147483647}, "pageYOffset": {-2147483648, 2147483647}}, "navigator": {"hardwareConcurrency": {1, 1024}, "maxTouchPoints": {0, 100}}}
	for section, fields := range ranges {
		o, _ := m[section].(map[string]any)
		for k, r := range fields {
			if v, exists := o[k]; exists {
				f, _ := number(v)
				if f < r[0] || f > r[1] {
					return nil, newError("validation")
				}
			}
		}
	}
	if effectiveProfileSize(m) > 64*1024 {
		return nil, newError("validation")
	}
	return Profile(m), nil
}
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
func effectiveProfileSize(m map[string]any) int {
	v, _ := strictJSON(defaultsJSON)
	effective := v.(map[string]any)
	for k, v := range m {
		if obj, ok := v.(map[string]any); ok {
			dst := effective[k].(map[string]any)
			for k, v := range obj {
				dst[k] = v
			}
		} else {
			effective[k] = v
		}
	}
	nav := effective["navigator"].(map[string]any)
	supplied, _ := m["navigator"].(map[string]any)
	for top, alias := range map[string]string{"user_agent": "userAgent", "platform": "platform", "language": "language", "languages": "languages"} {
		v, ok := supplied[alias]
		if !ok {
			v = effective[top]
		}
		effective[top] = v
		nav[alias] = v
	}
	if _, ok := supplied["appVersion"]; !ok {
		nav["appVersion"] = strings.TrimPrefix(text(effective["user_agent"]), "Mozilla/")
	}
	window := effective["window"].(map[string]any)
	if f, ok := number(window["devicePixelRatio"]); ok && math.Trunc(f) == f {
		window["devicePixelRatio"] = json.Number(strconv.FormatFloat(f, 'f', -1, 64) + ".0")
	}
	return canonicalSize(effective)
}
func textNumber(v any) string { return string(v.(json.Number)) }
func canonicalSize(v any) int {
	switch m := v.(type) {
	case map[string]any:
		n := 2
		for k, v := range m {
			b, _ := marshalJSON(k)
			n += len(b) + 2 + canonicalSize(v)
		}
		if len(m) > 0 {
			n += 2 * (len(m) - 1)
		}
		return n
	case []any:
		n := 2
		for _, v := range m {
			n += canonicalSize(v)
		}
		if len(m) > 0 {
			n += 2 * (len(m) - 1)
		}
		return n
	default:
		b, _ := marshalJSON(v)
		return len(b)
	}
}

func validProfileUTF8(v reflect.Value, depth int) bool {
	if depth > 64 {
		return false
	}
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return true
		}
		return validProfileUTF8(v.Elem(), depth+1)
	case reflect.String:
		return utf8.ValidString(v.String())
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			if !validProfileUTF8(it.Key(), depth+1) || !validProfileUTF8(it.Value(), depth+1) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !validProfileUTF8(v.Index(i), depth+1) {
				return false
			}
		}
	}
	return true
}
