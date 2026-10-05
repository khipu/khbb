package bitbucket

import (
	"encoding/json"
	"io"
)

// writeDryRun prints the request that would have been sent. Headers are never
// included, and any object with "secured": true has its "value" masked.
func writeDryRun(w io.Writer, method, url string, body []byte) error {
	out := map[string]any{"dryRun": true, "method": method, "url": url, "body": nil}
	if len(body) > 0 {
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			out["body"] = maskSecured(v)
		} else {
			out["body"] = string(body)
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func maskSecured(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if secured, _ := t["secured"].(bool); secured {
			if _, ok := t["value"]; ok {
				t["value"] = "****"
			}
		}
		for k, child := range t {
			t[k] = maskSecured(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = maskSecured(child)
		}
		return t
	}
	return v
}
