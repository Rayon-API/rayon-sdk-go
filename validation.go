package rayon

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var contentTypeRE = regexp.MustCompile(`^[\w!#$&^.+-]+/[\w!#$&^.+-]+(?:\s*;[^\r\n]*)?$`)
var formTypeRE = regexp.MustCompile(`(?i)^application/x-www-form-urlencoded(?:\s*;\s*charset\s*=\s*(?:"(?:utf-8|utf8)"|utf-8|utf8))?\s*$`)
var formNameRE = regexp.MustCompile("^([!#$%&'*+\\-.^_`|~0-9A-Za-z]{1,128})([abcdfz])$")
var headerNameRE = regexp.MustCompile("(?i)^([!#$%&'*+\\-.^_`|~0-9a-z]{1,128}-?)(a0|[abcdfz])$")

func controls(s string) bool {
	for _, c := range s {
		if c < 32 || c == 127 {
			return true
		}
	}
	return false
}
func validURL(s string) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > 8192 || strings.ContainsAny(s, "\\#") {
		return false
	}
	for _, c := range s {
		if c <= 32 || c == 127 {
			return false
		}
	}
	u, e := url.Parse(s)
	if e == nil && u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.Hostname() != "" && u.User == nil && u.Port() != "0"
}
func validateShape(s ShapeRequest) (ShapeRequest, error) {
	bad := func() (ShapeRequest, error) { return ShapeRequest{}, newError("validation") }
	if !validURL(s.SourceURL) || (s.Scripts != nil && s.MaterialRef != "") || (s.Mode == "form" && s.Scripts == nil && s.MaterialRef == "") {
		return bad()
	}
	if s.Scripts != nil {
		if len(s.Scripts) < 1 || len(s.Scripts) > 16 {
			return bad()
		}
		total := 0
		for _, script := range s.Scripts {
			if !validURL(script.URL) || script.Source == "" || !utf8.ValidString(script.Source) || len(script.Source) > 2*1024*1024 {
				return bad()
			}
			total += len(script.Source)
		}
		if total > 3*1024*1024 {
			return bad()
		}
	} else if s.MaterialRef != "" && !refRE.MatchString(s.MaterialRef) {
		return bad()
	}
	profile, e := ValidateProfile(s.Profile)
	if e != nil {
		return bad()
	}
	s.Profile = profile
	r := s.Request
	if !validURL(r.URL) || !strings.Contains("|GET|HEAD|POST|PUT|PATCH|DELETE|OPTIONS|", "|"+r.Method+"|") {
		return bad()
	}
	if r.ContentType != nil {
		ct := *r.ContentType
		if len(ct) > 256 || !utf8.ValidString(ct) || controls(ct) || !contentTypeRE.MatchString(ct) {
			return bad()
		}
	}
	if r.Body != nil {
		if len(*r.Body) > 256*1024 || !utf8.ValidString(*r.Body) || r.Method == "GET" || r.Method == "HEAD" || r.ContentType == nil {
			return bad()
		}
		ct := strings.ToLower(strings.Split(*r.ContentType, ";")[0])
		if ct == "application/octet-stream" || ct == "application/pdf" || ct == "application/zip" || strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "multipart/") {
			return bad()
		}
	}
	if s.Mode != "" && s.Mode != "header" && s.Mode != "form" {
		return bad()
	}
	if s.Mode != "form" {
		if s.Form != nil {
			return bad()
		}
	} else {
		if r.Method != "POST" || r.ContentType == nil || !formTypeRE.MatchString(*r.ContentType) || r.Body == nil {
			return bad()
		}
		if s.Form != nil {
			for _, v := range []*string{s.Form.ID, s.Form.Name} {
				if v != nil && (!utf8.ValidString(*v) || utf8.RuneCountInString(*v) > 256 || controls(*v)) {
					return bad()
				}
			}
		}
		pairs, e := formPairs(*r.Body)
		if e != nil {
			return bad()
		}
		families := map[string]map[string]bool{}
		for _, p := range pairs {
			m := formNameRE.FindStringSubmatch(p[0])
			if m != nil {
				if families[m[1]] == nil {
					families[m[1]] = map[string]bool{}
				}
				families[m[1]][m[2]] = true
			}
		}
		for _, slots := range families {
			if len(slots) == 6 {
				return bad()
			}
		}
	}
	if s.DocumentCookie != nil && (!utf8.ValidString(*s.DocumentCookie) || len(*s.DocumentCookie) > 16*1024 || controls(*s.DocumentCookie)) {
		return bad()
	}
	b, e := marshalJSON(s)
	if e != nil || len(b) > 4*1024*1024 {
		return bad()
	}
	s.Scripts = append([]ScriptMaterial(nil), s.Scripts...)
	return s, nil
}
func formPairs(body string) ([][2]string, error) {
	if len(body) > 256*1024 || !utf8.ValidString(body) {
		return nil, newError("validation")
	}
	pairs := [][2]string{}
	for _, part := range strings.Split(body, "&") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		k, e := url.QueryUnescape(kv[0])
		if e != nil {
			return nil, e
		}
		v := ""
		if len(kv) == 2 {
			v, e = url.QueryUnescape(kv[1])
			if e != nil {
				return nil, e
			}
		}
		if !utf8.ValidString(k) || !utf8.ValidString(v) || strings.ContainsRune(k, 0) || strings.ContainsRune(v, 0) || len(k) > 256 {
			return nil, newError("validation")
		}
		pairs = append(pairs, [2]string{k, v})
		if len(pairs) > 1024 {
			return nil, newError("validation")
		}
	}
	return pairs, nil
}
func formEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("*-._", rune(c)) {
			b.WriteByte(c)
		} else if c == ' ' {
			b.WriteByte('+')
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
