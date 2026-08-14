package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

func Sign(secret, rawBody string, timestampSec int64) (header, signature string) {
	if timestampSec == 0 {
		timestampSec = time.Now().UTC().Unix()
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(timestampSec, 10) + "." + rawBody))
	signature = hex.EncodeToString(mac.Sum(nil))
	header = "t=" + strconv.FormatInt(timestampSec, 10) + ",v1=" + signature
	return header, signature
}

func ParseSignatureHeader(header string) (timestampSec int64, signature string, ok bool) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		key, val, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		switch key {
		case "t":
			n, err := strconv.ParseInt(val, 10, 64)
			if err == nil {
				timestampSec = n
			}
		case "v1":
			signature = val
		}
	}
	return timestampSec, signature, timestampSec != 0 && signature != ""
}

func Verify(secret, rawBody, header string, nowSec int64, toleranceSec int64) bool {
	ts, sig, ok := ParseSignatureHeader(header)
	if !ok {
		return false
	}
	if toleranceSec <= 0 {
		toleranceSec = 300
	}
	if nowSec == 0 {
		nowSec = time.Now().UTC().Unix()
	}
	delta := nowSec - ts
	if delta < 0 {
		delta = -delta
	}
	if delta > toleranceSec {
		return false
	}
	_, expected := Sign(secret, rawBody, ts)
	if len(expected) != len(sig) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) == 1
}
