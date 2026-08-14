package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	tokenPrefix = "fnk_live_"
	prefixBytes = 8
	secretBytes = 24
)

type Parsed struct {
	Prefix string
	Secret string
}

func Parse(token string) (Parsed, error) {
	token = strings.TrimSpace(token)
	rest, ok := strings.CutPrefix(token, tokenPrefix)
	if !ok {
		return Parsed{}, ErrInvalidToken
	}
	prefix, secret, ok := strings.Cut(rest, "_")
	if !ok || prefix == "" || secret == "" {
		return Parsed{}, ErrInvalidToken
	}
	if len(prefix) != prefixBytes*2 {
		return Parsed{}, ErrInvalidToken
	}
	for _, c := range prefix {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return Parsed{}, ErrInvalidToken
		}
	}
	return Parsed{Prefix: prefix, Secret: secret}, nil
}

func HashSecret(pepper, secret string) string {
	sum := sha256.Sum256([]byte(pepper + secret))
	return hex.EncodeToString(sum[:])
}

func Verify(pepper, secret, secretHash string) bool {
	got := HashSecret(pepper, secret)
	if len(got) != len(secretHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(secretHash)) == 1
}

func Format(prefix, secret string) string {
	return tokenPrefix + prefix + "_" + secret
}

func generate() (prefix, secret string, err error) {
	p := make([]byte, prefixBytes)
	if _, err := rand.Read(p); err != nil {
		return "", "", fmt.Errorf("prefix: %w", err)
	}
	s := make([]byte, secretBytes)
	if _, err := rand.Read(s); err != nil {
		return "", "", fmt.Errorf("secret: %w", err)
	}
	return hex.EncodeToString(p), base64.RawURLEncoding.EncodeToString(s), nil
}
