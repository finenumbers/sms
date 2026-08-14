package smsc

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testProvider(t *testing.T, in Input, rt http.RoundTripper, store Persistence) *Provider {
	t.Helper()
	cfg, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := testHTTP(cfg, rt, nil)
	return New(Options{Config: cfg, Persistence: store, HTTP: httpClient})
}

func TestProviderSubmitHLRPersistsRedacted(t *testing.T) {
	raw := loadFixture(t, "send-hlr-success.json")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(raw, 200), nil
	})
	mem := NewMemory()
	p := testProvider(t, Input{Login: "u", Password: "p", Currency: "RUB"}, rt, mem)
	result, err := p.SubmitHLR(context.Background(), SubmitInput{
		PhoneE164:      "+79991234567",
		IdempotencyKey: "item-1",
		TenantID:       "tenant-1",
		JobItemID:      "job-item-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderMessageID != "1001" || !result.Accepted || result.Deduplicated {
		t.Fatalf("%#v", result)
	}
	if result.Normalized.LifecycleStatus != LifecycleAccepted {
		t.Fatalf("lifecycle %s", result.Normalized.LifecycleStatus)
	}
	stored := mem.Requests()
	if len(stored) != 1 {
		t.Fatalf("requests %d", len(stored))
	}
	row := stored[0]
	if row.Status != RequestSucceeded {
		t.Fatalf("status %s", row.Status)
	}
	got, _ := json.Marshal(row.ResponsePayload)
	want, _ := json.Marshal(raw)
	if string(got) != string(want) {
		t.Fatalf("response %s != %s", got, want)
	}
	if row.Normalized == nil || row.Normalized.LifecycleStatus != LifecycleAccepted || row.Normalized.ProviderMessageID != "1001" {
		t.Fatalf("normalized %#v", row.Normalized)
	}
	blob, _ := json.Marshal(row.RequestPayload)
	if bytes.Contains(blob, []byte("secret")) {
		t.Fatalf("request leaked secret: %s", blob)
	}
	req, _ := row.RequestPayload.(map[string]any)
	if _, ok := req["psw"]; ok {
		t.Fatal("psw must not be persisted")
	}
}

func TestProviderDeduplicatesSuccessfulSubmit(t *testing.T) {
	var n atomic.Int32
	raw := loadFixture(t, "send-hlr-success.json")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		n.Add(1)
		return jsonResponse(raw, 200), nil
	})
	p := testProvider(t, Input{Login: "u", Password: "p"}, rt, NewMemory())
	in := SubmitInput{PhoneE164: "+79991234567", IdempotencyKey: "item-1", TenantID: "tenant-1", JobItemID: "job-item-1"}
	if _, err := p.SubmitHLR(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	second, err := p.SubmitHLR(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Deduplicated || n.Load() != 1 {
		t.Fatalf("dedup=%v calls=%d", second.Deduplicated, n.Load())
	}
}

func TestProviderBlocksInFlightSubmit(t *testing.T) {
	var once sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	raw := loadFixture(t, "send-hlr-success.json")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		<-release
		return jsonResponse(raw, 200), nil
	})
	p := testProvider(t, Input{Login: "u", Password: "p"}, rt, NewMemory())
	in := SubmitInput{PhoneE164: "+79991234567", IdempotencyKey: "item-inflight", TenantID: "tenant-1", JobItemID: "job-item-1"}

	errCh := make(chan error, 1)
	go func() {
		_, err := p.SubmitHLR(context.Background(), in)
		errCh <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first submit did not start")
	}
	_, err := p.SubmitHLR(context.Background(), in)
	pe := AsError(err)
	if pe == nil || pe.Kind != KindConflict || !pe.Retryable {
		t.Fatalf("expected conflict, got %#v", pe)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestProviderRetryAfterFailedSubmit(t *testing.T) {
	var n atomic.Int32
	auth := loadFixture(t, "send-error-auth.json")
	ok := loadFixture(t, "send-hlr-success.json")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if n.Add(1) == 1 {
			return jsonResponse(auth, 200), nil
		}
		return jsonResponse(ok, 200), nil
	})
	p := testProvider(t, Input{Login: "u", Password: "p"}, rt, NewMemory())
	in := SubmitInput{PhoneE164: "+79991234567", IdempotencyKey: "item-retry", TenantID: "tenant-1", JobItemID: "job-item-1"}
	if err := firstErr(p.SubmitHLR(context.Background(), in)); AsError(err) == nil {
		t.Fatal("expected first submit to fail")
	}
	second, err := p.SubmitHLR(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if second.Deduplicated || second.ProviderMessageID != "1001" || n.Load() != 2 {
		t.Fatalf("second %#v calls=%d", second, n.Load())
	}
}

func TestProviderEstimateHLRCost(t *testing.T) {
	raw := loadFixture(t, "cost-hlr.json")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(raw, 200), nil
	})
	p := testProvider(t, Input{Login: "u", Password: "p", Currency: "RUB"}, rt, nil)
	est, err := p.EstimateHLRCost(context.Background(), SubmitInput{PhoneE164: "+79991234567"})
	if err != nil {
		t.Fatal(err)
	}
	if est.Cost != "0.30" || est.Parts == nil || *est.Parts != 1 || est.CheckType != CheckHLR {
		t.Fatalf("%#v", est)
	}
}

func TestProviderAuthErrorCode(t *testing.T) {
	raw := loadFixture(t, "send-error-auth.json")
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(raw, 200), nil
	})
	p := testProvider(t, Input{Login: "u", Password: "p"}, rt, nil)
	_, err := p.SubmitPing(context.Background(), SubmitInput{
		PhoneE164: "+79991234567", IdempotencyKey: "x", TenantID: "t", JobItemID: "j",
	})
	pe := AsError(err)
	if pe == nil || pe.ProviderErrorCode != 2 || pe.Kind != KindAuth {
		t.Fatalf("%#v", pe)
	}
}

func TestProviderCallbackRequiresSecret(t *testing.T) {
	p := testProvider(t, Input{Login: "u", Password: "p", CallbackSecret: ""}, nil, NewMemory())
	raw := loadFixture(t, "callback-hlr.json")
	_, err := p.HandleCallback(context.Background(), CallbackInput{RawPayload: raw})
	pe := AsError(err)
	if pe == nil || pe.Kind != KindSignature || pe.Message != "SMSC callback secret is not configured" {
		t.Fatalf("%#v", pe)
	}
}

func TestProviderCallbackNormalizesAndDedupes(t *testing.T) {
	secret := "callback-secret"
	p := testProvider(t, Input{Login: "u", Password: "p", CallbackSecret: secret}, nil, NewMemory())
	raw := loadFixture(t, "callback-hlr.json")
	base := asString(raw["id"]) + ":" + asString(raw["phone"]) + ":" + asString(raw["status"]) + ":" + secret
	sum := md5.Sum([]byte(base))
	raw["md5"] = hex.EncodeToString(sum[:])

	result, err := p.HandleCallback(context.Background(), CallbackInput{RawPayload: raw})
	if err != nil {
		t.Fatal(err)
	}
	if result.Normalized.ResultStatus != ResultReachable || result.SignatureValid == nil || !*result.SignatureValid || result.Deduplicated {
		t.Fatalf("%#v", result)
	}
	stored := p.store.(*Memory).Callbacks()
	if len(stored) != 1 || stored[0].Normalized == nil || stored[0].Normalized.ResultStatus != ResultReachable {
		t.Fatalf("stored %#v", stored)
	}
	if stored[0].Normalized.IMSI != "250011234567890" {
		t.Fatalf("imsi %s", stored[0].Normalized.IMSI)
	}
	again, err := p.HandleCallback(context.Background(), CallbackInput{RawPayload: raw})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Deduplicated {
		t.Fatal("expected deduplicated callback")
	}
}

type ctxProbeStore struct {
	*Memory
	saveReq  error
	update   error
	saveCB   error
	findLast error
}

func (s *ctxProbeStore) SaveRequest(ctx context.Context, record RequestRecord) (SaveResult, error) {
	s.saveReq = ctx.Err()
	return s.Memory.SaveRequest(ctx, record)
}

func (s *ctxProbeStore) UpdateRequest(ctx context.Context, id string, patch RequestPatch) error {
	s.update = ctx.Err()
	return s.Memory.UpdateRequest(ctx, id, patch)
}

func (s *ctxProbeStore) FindLatestSend(ctx context.Context, providerCode, tenantID, idempotencyKey string) (*RequestRecord, error) {
	s.findLast = ctx.Err()
	return s.Memory.FindLatestSend(ctx, providerCode, tenantID, idempotencyKey)
}

func (s *ctxProbeStore) SaveCallback(ctx context.Context, record CallbackRecord) (SaveResult, error) {
	s.saveCB = ctx.Err()
	return s.Memory.SaveCallback(ctx, record)
}

func TestProviderCallbackPersistsAfterCancel(t *testing.T) {
	secret := "callback-secret"
	probe := &ctxProbeStore{Memory: NewMemory()}
	p := testProvider(t, Input{Login: "u", Password: "p", CallbackSecret: secret}, nil, probe)
	raw := loadFixture(t, "callback-hlr.json")
	base := asString(raw["id"]) + ":" + asString(raw["phone"]) + ":" + asString(raw["status"]) + ":" + secret
	sum := md5.Sum([]byte(base))
	raw["md5"] = hex.EncodeToString(sum[:])

	parent, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := p.HandleCallback(parent, CallbackInput{RawPayload: raw})
	if err != nil {
		t.Fatal(err)
	}
	if result.Normalized.ResultStatus != ResultReachable {
		t.Fatalf("%#v", result)
	}
	if probe.saveCB != nil {
		t.Fatalf("callback persist must use WithoutCancel, got %v", probe.saveCB)
	}
	if len(probe.Callbacks()) != 1 {
		t.Fatal("cancelled ingress ctx must not drop the callback row")
	}
}

func TestProviderSaveRequestSeesLiveCancel(t *testing.T) {
	probe := &ctxProbeStore{Memory: NewMemory()}
	p := testProvider(t, Input{Login: "u", Password: "p"}, nil, probe)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.saveRequest(parent, RequestRecord{
		ProviderCode: ProviderCode,
		Kind:         KindBalance,
		Status:       RequestPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.saveReq == nil {
		t.Fatal("SaveRequest before HTTP must see the live Tick ctx")
	}
}

func firstErr[T any](v T, err error) error { return err }
