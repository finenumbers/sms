package ingress

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const MaxBody = 1 << 20

func IdempotencyKey(method, path, query string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(strings.ToUpper(method)))
	h.Write([]byte{'\n'})
	h.Write([]byte(path))
	h.Write([]byte{'\n'})
	h.Write([]byte(query))
	h.Write([]byte{'\n'})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TokenMatch(storedHash, token string) bool {
	if storedHash == "" || token == "" {
		return false
	}
	got := HashToken(token)
	if len(got) != len(storedHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}

func RedactPath(path string) string {
	const prefix = "/internal/runexis/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	rest := strings.TrimPrefix(path, prefix)
	kind, _, ok := strings.Cut(rest, "/")
	if !ok || (kind != "dlr" && kind != "mo") {
		return prefix + "*"
	}
	return prefix + kind + "/*"
}

func SanitizeHeaders(h http.Header) []byte {
	out := map[string][]string{}
	for k, v := range h {
		lk := strings.ToLower(k)
		if lk == "cookie" || lk == "authorization" {
			continue
		}
		out[k] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func ReadBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
}

func KindFromPath(kind string) (sqlcdb.CallbackKind, bool) {
	switch strings.ToLower(kind) {
	case "dlr":
		return sqlcdb.CallbackKindDlr, true
	case "mo":
		return sqlcdb.CallbackKindMo, true
	default:
		return "", false
	}
}
