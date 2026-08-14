package smsc

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMapResponseSendAck(t *testing.T) {
	raw := loadFixture(t, "send-hlr-success.json")
	result := MapResponse(MapResponseInput{
		CheckType: CheckHLR,
		Raw:       raw,
		PhoneE164: "+79991234567",
		Currency:  "RUB",
	})
	if result.LifecycleStatus != LifecycleAccepted || result.ResultStatus != ResultPending {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.ProviderMessageID != "1001" || result.Cost != "0.30" {
		t.Fatalf("id=%s cost=%s", result.ProviderMessageID, result.Cost)
	}
	if result.IsReachable != nil {
		t.Fatal("expected null reachability")
	}
}

func TestMapResponseHLRReachable(t *testing.T) {
	raw := loadFixture(t, "status-hlr-reachable.json")
	result := MapResponse(MapResponseInput{CheckType: CheckHLR, Raw: raw, Currency: "RUB"})
	if result.LifecycleStatus != LifecycleCompleted || result.ResultStatus != ResultReachable {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.IsReachable == nil || !*result.IsReachable {
		t.Fatal("expected reachable")
	}
	if result.IMSI != "250011234567890" || result.MCC != "250" || result.MNC != "01" {
		t.Fatalf("imsi/mcc/mnc: %s %s %s", result.IMSI, result.MCC, result.MNC)
	}
	if result.OperatorName != "MTS" || result.CountryCode != "Russia" {
		t.Fatalf("op=%s cn=%s", result.OperatorName, result.CountryCode)
	}
	if result.PhoneE164 != "+79991234567" {
		t.Fatalf("phone %s", result.PhoneE164)
	}
	if result.Extras["msc"] != "79001234567" || result.Extras["region"] != "Moscow" {
		t.Fatalf("extras %#v", result.Extras)
	}
	if result.Roaming == nil || *result.Roaming {
		t.Fatal("expected roaming=false")
	}
}

func TestMapResponseHLRUnreachable(t *testing.T) {
	raw := loadFixture(t, "status-hlr-unreachable.json")
	result := MapResponse(MapResponseInput{CheckType: CheckHLR, Raw: raw})
	if result.LifecycleStatus != LifecycleCompleted || result.ResultStatus != ResultUnreachable {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.IsReachable == nil || *result.IsReachable {
		t.Fatal("expected unreachable")
	}
	if result.ProviderErrorCode != "23" || result.ProviderStatusCode != "1" {
		t.Fatalf("err=%s status=%s", result.ProviderErrorCode, result.ProviderStatusCode)
	}
}

func TestMapResponsePending(t *testing.T) {
	raw := loadFixture(t, "status-pending.json")
	result := MapResponse(MapResponseInput{CheckType: CheckHLR, Raw: raw})
	if result.LifecycleStatus != LifecyclePending || result.ResultStatus != ResultPending {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.IsReachable != nil {
		t.Fatal("expected null reachability")
	}
}

func TestMapResponseAPIError(t *testing.T) {
	raw := loadFixture(t, "send-error-auth.json")
	result := MapResponse(MapResponseInput{CheckType: CheckPing, Raw: raw})
	if result.LifecycleStatus != LifecycleFailed || result.ResultStatus != ResultError {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.ProviderErrorCode != "2" {
		t.Fatalf("code %s", result.ProviderErrorCode)
	}
	if result.ProviderErrorMessage == "" || result.ProviderErrorMessage != "invalid login or password" {
		t.Fatalf("msg %q", result.ProviderErrorMessage)
	}
}

func TestMapResponseCallback(t *testing.T) {
	raw := loadFixture(t, "callback-hlr.json")
	result := MapResponse(MapResponseInput{CheckType: CheckHLR, Raw: raw})
	if result.LifecycleStatus != LifecycleCompleted || result.ResultStatus != ResultReachable {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.ProviderMessageID != "1001" || result.IMSI != "250011234567890" {
		t.Fatalf("id=%s imsi=%s", result.ProviderMessageID, result.IMSI)
	}
}

func TestMapStatusMinus3(t *testing.T) {
	result := MapStatus(MapStatusInput{CheckType: CheckHLR, StatusCode: -3, ProviderMessageID: "9"})
	if result.LifecycleStatus != LifecycleFailed || result.ResultStatus != ResultError {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
}

func TestMapStatusDeliveryFailure(t *testing.T) {
	result := MapStatus(MapStatusInput{CheckType: CheckPing, StatusCode: 20, ProviderMessageID: "9"})
	if result.LifecycleStatus != LifecycleCompleted || result.ResultStatus != ResultUnreachable {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.IsReachable == nil || *result.IsReachable {
		t.Fatal("expected unreachable")
	}
}

func TestMapStatus12WithoutErrStaysPending(t *testing.T) {
	result := MapStatus(MapStatusInput{CheckType: CheckHLR, StatusCode: 1, ProviderMessageID: "9"})
	if result.LifecycleStatus != LifecyclePending || result.ResultStatus != ResultPending {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.IsReachable != nil {
		t.Fatal("do not guess reachable")
	}
}

func TestMapStatusErrOnly(t *testing.T) {
	errOnly := MapStatus(MapStatusInput{CheckType: CheckHLR, StatusCode: nil, ErrorCode: 23, ProviderMessageID: "9"})
	if errOnly.LifecycleStatus != LifecycleCompleted || errOnly.ResultStatus != ResultUnreachable {
		t.Fatalf("err-only: %s %s", errOnly.LifecycleStatus, errOnly.ResultStatus)
	}
	errZero := MapStatus(MapStatusInput{CheckType: CheckHLR, StatusCode: nil, ErrorCode: 0, ProviderMessageID: "9"})
	if errZero.LifecycleStatus != LifecycleCompleted || errZero.ResultStatus != ResultReachable {
		t.Fatalf("err-zero: %s %s", errZero.LifecycleStatus, errZero.ResultStatus)
	}
}

func TestMapResponseNonJSON(t *testing.T) {
	result := MapResponse(MapResponseInput{
		CheckType: CheckHLR,
		Raw:       map[string]any{"_nonJson": true, "text": "ERROR temporary"},
	})
	if result.LifecycleStatus != LifecycleFailed || result.ResultStatus != ResultError {
		t.Fatalf("lifecycle=%s result=%s", result.LifecycleStatus, result.ResultStatus)
	}
	if result.ProviderErrorMessage == "" || result.ProviderErrorMessage[:8] != "Non-JSON" {
		t.Fatalf("msg %q", result.ProviderErrorMessage)
	}
}

func TestCallbackDedupeKey(t *testing.T) {
	a := loadFixture(t, "callback-hlr.json")
	b := cloneMap(a)
	if CallbackDedupeKey(a) != CallbackDedupeKey(b) {
		t.Fatal("expected stable key")
	}
	changed := cloneMap(a)
	changed["status"] = "20"
	if CallbackDedupeKey(changed) == CallbackDedupeKey(a) {
		t.Fatal("status change must change key")
	}
}

func TestVerifyCallbackSignature(t *testing.T) {
	secret := "test-secret"
	payload := map[string]any{"id": "1001", "phone": "79991234567", "status": "1"}
	sum := md5.Sum([]byte("1001:79991234567:1:" + secret))
	md5hex := hex.EncodeToString(sum[:])

	if v := VerifyCallbackSignature(VerifyInput{Payload: payload, Secret: secret, Signatures: CallbackSignatures{MD5: md5hex}}); v == nil || !*v {
		t.Fatal("expected valid md5")
	}
	if v := VerifyCallbackSignature(VerifyInput{Payload: payload, Secret: secret, Signatures: CallbackSignatures{MD5: "deadbeef"}}); v == nil || *v {
		t.Fatal("expected invalid md5")
	}
	if v := VerifyCallbackSignature(VerifyInput{Payload: payload, Secret: ""}); v != nil {
		t.Fatal("empty secret must return nil")
	}
}

func TestClientIDFromKey(t *testing.T) {
	a := ClientIDFromKey(SendIdempotencyKey(CheckHLR, "item-1"))
	b := ClientIDFromKey(SendIdempotencyKey(CheckHLR, "item-1"))
	c := ClientIDFromKey(SendIdempotencyKey(CheckPing, "item-1"))
	if a != b || a <= 0 || a > 0x7fffffff {
		t.Fatalf("stable 31-bit id: %d %d", a, b)
	}
	if a == c {
		t.Fatal("hlr and ping keys must differ")
	}
	if SendIdempotencyKey(CheckHLR, "item-1") != "SEND:hlr:item-1" {
		t.Fatalf("key %s", SendIdempotencyKey(CheckHLR, "item-1"))
	}
}
