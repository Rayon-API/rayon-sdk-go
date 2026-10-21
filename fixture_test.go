package rayon

import (
	"os"
	"strings"
)

func osReadFixture() ([]byte, error) { return os.ReadFile("testdata/profile-contract.json") }

func expandFixture(v any) any {
	switch x := v.(type) {
	case Profile:
		return expandFixture(map[string]any(x))
	case map[string]any:
		if marker, ok := x["$repeat"].(map[string]any); ok {
			return strings.Repeat(marker["text"].(string), int(marker["count"].(float64)))
		}
		o := map[string]any{}
		for k, v := range x {
			o[k] = expandFixture(v)
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i, v := range x {
			o[i] = expandFixture(v)
		}
		return o
	}
	return v
}
