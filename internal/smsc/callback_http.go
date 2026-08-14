package smsc

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// MergeCallbackPayload merges GET query with POST body. Body keys win.
func MergeCallbackPayload(query url.Values, body []byte, contentType string) map[string]any {
	out := map[string]any{}
	for k, vs := range query {
		if len(vs) == 0 {
			continue
		}
		out[k] = vs[0]
	}
	for k, v := range parseCallbackBody(body, contentType) {
		out[k] = v
	}
	return out
}

func SignaturesFromRequest(r *http.Request, payload map[string]any) CallbackSignatures {
	return CallbackSignatures{
		MD5:   firstNonEmpty(headerOrEmpty(r, "X-SMSC-MD5"), asString(payload["md5"])),
		SHA1:  firstNonEmpty(headerOrEmpty(r, "X-SMSC-SHA1"), asString(payload["sha1"])),
		CRC32: asString(payload["crc32"]),
	}
}

func parseCallbackBody(body []byte, contentType string) map[string]any {
	if len(body) == 0 {
		return nil
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case ct == "application/json" || (ct == "" && looksJSON(body)):
		var raw any
		if json.Unmarshal(body, &raw) != nil {
			return nil
		}
		if obj, ok := asObject(raw); ok {
			return obj
		}
		return nil
	default:
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return nil
		}
		out := map[string]any{}
		for k, vs := range vals {
			if len(vs) > 0 {
				out[k] = vs[0]
			}
		}
		return out
	}
}

func looksJSON(body []byte) bool {
	s := strings.TrimSpace(string(body))
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

func headerOrEmpty(r *http.Request, key string) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(key))
}
