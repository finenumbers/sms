package smsc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

type ConfigSource interface {
	SMSCConfig(ctx context.Context) (Config, error)
}

type Provider struct {
	cfg    Config
	source ConfigSource
	store  Persistence
	log    *slog.Logger
	http   *HTTPClient
}

type Options struct {
	Config      Config
	Source      ConfigSource
	Persistence Persistence
	Log         *slog.Logger
	HTTP        *HTTPClient
}

func (p *Provider) liveConfig(ctx context.Context) Config {
	if p == nil {
		return Config{}
	}
	if p.source != nil {
		cfg, err := p.source.SMSCConfig(ctx)
		if err == nil {
			return cfg
		}
		if p.log != nil {
			p.log.Error("smsc settings", "err", err)
		}
	}
	return p.cfg
}

func (p *Provider) Configured() bool {
	return p != nil && p.liveConfig(context.Background()).Configured()
}

func (p *Provider) CallbackSecretConfigured() bool {
	return p != nil && strings.TrimSpace(p.liveConfig(context.Background()).CallbackSecret) != ""
}

func New(opts Options) *Provider {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	httpClient := opts.HTTP
	if httpClient == nil {
		httpClient = NewHTTPClient(HTTPOptions{Config: opts.Config, Source: opts.Source, Log: log})
	}
	return &Provider{cfg: opts.Config, source: opts.Source, store: opts.Persistence, log: log, http: httpClient}
}

func (p *Provider) EstimateHLRCost(ctx context.Context, in SubmitInput) (CostEstimate, error) {
	return p.estimateCost(ctx, CheckHLR, in)
}

func (p *Provider) EstimatePingCost(ctx context.Context, in SubmitInput) (CostEstimate, error) {
	return p.estimateCost(ctx, CheckPing, in)
}

func (p *Provider) SubmitHLR(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	return p.submit(ctx, CheckHLR, in)
}

func (p *Provider) SubmitPing(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	return p.submit(ctx, CheckPing, in)
}

func (p *Provider) FetchStatus(ctx context.Context, in FetchStatusInput) (FetchStatusResult, error) {
	phone := ToPhoneDigits(in.PhoneE164)
	all := 2
	if in.IncludeDetails != nil && !*in.IncludeDetails {
		all = 0
	}
	requestPayload := RedactSecrets(map[string]any{
		"path":      "/sys/status.php",
		"phone":     phone,
		"id":        in.ProviderMessageID,
		"all":       all,
		"checkType": in.CheckType,
	})
	startedAt := time.Now().UTC()
	saved, err := p.saveRequest(ctx, RequestRecord{
		TenantID:          in.TenantID,
		JobItemID:         in.JobItemID,
		ProviderCode:      ProviderCode,
		Kind:              KindStatus,
		Status:            RequestPending,
		ProviderMessageID: in.ProviderMessageID,
		RequestPayload:    requestPayload,
		StartedAt:         startedAt,
	})
	if err != nil {
		return FetchStatusResult{}, err
	}

	result, err := p.http.Request(ctx, "/sys/status.php", map[string]any{
		"phone": phone,
		"id":    in.ProviderMessageID,
		"all":   all,
	}, in.CorrelationID, string(KindStatus))
	if err != nil {
		_ = p.failRequest(ctx, saved.ID, err, &mapFail{
			CheckType:         in.CheckType,
			PhoneE164:         in.PhoneE164,
			ProviderMessageID: in.ProviderMessageID,
		})
		observeOutcome(KindStatus, err)
		return FetchStatusResult{}, err
	}
	if err := assertNoError(result.Body, in.CorrelationID, result.HTTPStatus); err != nil {
		_ = p.failRequest(ctx, saved.ID, err, &mapFail{
			CheckType:         in.CheckType,
			PhoneE164:         in.PhoneE164,
			ProviderMessageID: in.ProviderMessageID,
		})
		observeOutcome(KindStatus, err)
		return FetchStatusResult{}, err
	}

	normalized := p.MapResponse(MapResponseInput{
		CheckType:         in.CheckType,
		Raw:               result.Body,
		PhoneE164:         in.PhoneE164,
		ProviderMessageID: in.ProviderMessageID,
	})
	if saved.ID != "" {
		cp := normalized
		_ = p.updateRequest(ctx, saved.ID, RequestPatch{
			Status:            RequestSucceeded,
			HTTPStatus:        result.HTTPStatus,
			ResponsePayload:   RedactSecrets(result.Body),
			Normalized:        &cp,
			FinishedAt:        time.Now().UTC(),
			ProviderMessageID: normalized.ProviderMessageID,
		})
	}
	observeOutcome(KindStatus, nil)
	return FetchStatusResult{
		ProviderCode:      ProviderCode,
		ProviderMessageID: in.ProviderMessageID,
		Normalized:        normalized,
		RawRequest:        requestPayload,
		RawResponse:       result.Body,
		ProviderRequestID: saved.ID,
	}, nil
}

func (p *Provider) HandleCallback(ctx context.Context, in CallbackInput) (CallbackResult, error) {
	payload, _ := asObject(in.RawPayload)
	if payload == nil {
		payload = map[string]any{}
	}
	checkType := InferCheckType(payload)
	signatureValid := VerifyCallbackSignature(VerifyInput{
		Payload:    payload,
		Secret:     p.liveConfig(ctx).CallbackSecret,
		Signatures: in.Signatures,
	})
	if signatureValid == nil || !*signatureValid {
		msg := "Invalid callback signature"
		kindMsg := "Invalid SMSC callback signature"
		if signatureValid == nil {
			msg = "SMSC callback secret is not configured"
			kindMsg = msg
		}
		var stored *bool
		if signatureValid != nil {
			stored = boolPtr(false)
		}
		_, _ = p.saveCallback(ctx, CallbackRecord{
			TenantID:          in.TenantID,
			JobItemID:         in.JobItemID,
			ProviderCode:      ProviderCode,
			ProviderMessageID: asString(payload["id"]),
			RawPayload:        RedactSecrets(in.RawPayload),
			SignatureValid:    stored,
			DedupeKey:         CallbackDedupeKey(payload),
			ProcessError:      msg,
		})
		return CallbackResult{}, &Error{
			ProviderCode:  ProviderCode,
			Kind:          KindSignature,
			Message:       kindMsg,
			Retryable:     false,
			CorrelationID: in.CorrelationID,
			RawResponse:   RedactSecrets(in.RawPayload),
		}
	}

	phone := CanonicalPhoneE164(CallbackPhoneRaw(payload))
	normalized := p.MapResponse(MapResponseInput{
		CheckType:         checkType,
		Raw:               payload,
		PhoneE164:         phone,
		ProviderMessageID: asString(payload["id"]),
	})
	cp := normalized
	saved, err := p.saveCallback(ctx, CallbackRecord{
		TenantID:          in.TenantID,
		JobItemID:         in.JobItemID,
		ProviderCode:      ProviderCode,
		ProviderMessageID: normalized.ProviderMessageID,
		RawPayload:        RedactSecrets(in.RawPayload),
		Normalized:        &cp,
		SignatureValid:    signatureValid,
		DedupeKey:         CallbackDedupeKey(payload),
	})
	if err != nil {
		return CallbackResult{}, err
	}
	p.log.Info("smsc.callback.normalized",
		"providerMessageId", normalized.ProviderMessageID,
		"lifecycleStatus", normalized.LifecycleStatus,
		"resultStatus", normalized.ResultStatus,
		"signatureValid", true,
		"deduplicated", saved.Deduplicated,
		"correlationId", in.CorrelationID,
	)
	return CallbackResult{
		ProviderCode:       ProviderCode,
		ProviderMessageID:  normalized.ProviderMessageID,
		SignatureValid:     signatureValid,
		Deduplicated:       saved.Deduplicated,
		Normalized:         normalized,
		RawPayload:         in.RawPayload,
		ProviderCallbackID: saved.ID,
	}, nil
}

func (p *Provider) MapResponse(in MapResponseInput) NormalizedResult {
	in.Currency = p.liveConfig(context.Background()).Currency
	return MapResponse(in)
}

func (p *Provider) MapStatus(in MapStatusInput) NormalizedResult {
	in.Currency = p.liveConfig(context.Background()).Currency
	return MapStatus(in)
}

func (p *Provider) Balance(ctx context.Context, correlationID string) (Balance, error) {
	requestPayload := RedactSecrets(map[string]any{"path": "/sys/balance.php"})
	saved, err := p.saveRequest(ctx, RequestRecord{
		ProviderCode:   ProviderCode,
		Kind:           KindBalance,
		Status:         RequestPending,
		RequestPayload: requestPayload,
		StartedAt:      time.Now().UTC(),
	})
	if err != nil {
		return Balance{}, err
	}
	result, err := p.http.Request(ctx, "/sys/balance.php", map[string]any{}, correlationID, string(KindBalance))
	if err != nil {
		_ = p.failRequest(ctx, saved.ID, err, nil)
		observeOutcome(KindBalance, err)
		return Balance{}, err
	}
	if err := assertNoError(result.Body, correlationID, result.HTTPStatus); err != nil {
		_ = p.failRequest(ctx, saved.ID, err, nil)
		observeOutcome(KindBalance, err)
		return Balance{}, err
	}
	obj, _ := asObject(result.Body)
	bal := asString(obj["balance"])
	if bal == "" {
		err := &Error{
			ProviderCode:  ProviderCode,
			Kind:          KindProvider,
			Message:       "SMSC balance response missing balance field",
			Retryable:     false,
			CorrelationID: correlationID,
			RawResponse:   result.Body,
		}
		_ = p.failRequest(ctx, saved.ID, err, nil)
		observeOutcome(KindBalance, err)
		return Balance{}, err
	}
	if saved.ID != "" {
		_ = p.updateRequest(ctx, saved.ID, RequestPatch{
			Status:          RequestSucceeded,
			HTTPStatus:      result.HTTPStatus,
			ResponsePayload: RedactSecrets(result.Body),
			FinishedAt:      time.Now().UTC(),
		})
	}
	observeOutcome(KindBalance, nil)
	return Balance{
		ProviderCode: ProviderCode,
		Balance:      bal,
		Currency:     p.liveConfig(ctx).Currency,
		RawResponse:  result.Body,
	}, nil
}

func (p *Provider) estimateCost(ctx context.Context, checkType CheckType, in SubmitInput) (CostEstimate, error) {
	phone := ToPhoneDigits(in.PhoneE164)
	flags := checkTypeFlags(checkType)
	payload := map[string]any{
		"path":      "/sys/send.php",
		"phones":    phone,
		"cost":      1,
		"checkType": checkType,
	}
	for k, v := range flags {
		payload[k] = v
	}
	requestPayload := RedactSecrets(payload)
	saved, err := p.saveRequest(ctx, RequestRecord{
		TenantID:       in.TenantID,
		JobItemID:      in.JobItemID,
		ProviderCode:   ProviderCode,
		Kind:           KindCost,
		Status:         RequestPending,
		RequestPayload: requestPayload,
		StartedAt:      time.Now().UTC(),
	})
	if err != nil {
		return CostEstimate{}, err
	}
	params := map[string]any{"phones": phone, "cost": 1}
	for k, v := range flags {
		params[k] = v
	}
	result, err := p.http.Request(ctx, "/sys/send.php", params, in.CorrelationID, string(KindCost))
	if err != nil {
		_ = p.failRequest(ctx, saved.ID, err, nil)
		observeOutcome(KindCost, err)
		return CostEstimate{}, err
	}
	if err := assertNoError(result.Body, in.CorrelationID, result.HTTPStatus); err != nil {
		_ = p.failRequest(ctx, saved.ID, err, nil)
		observeOutcome(KindCost, err)
		return CostEstimate{}, err
	}
	obj, _ := asObject(result.Body)
	if obj == nil || !hasNonEmpty(obj, "cost") {
		err := &Error{
			ProviderCode:  ProviderCode,
			Kind:          KindProvider,
			Message:       "SMSC cost response missing cost field",
			Retryable:     false,
			CorrelationID: in.CorrelationID,
			RawResponse:   result.Body,
		}
		_ = p.failRequest(ctx, saved.ID, err, nil)
		observeOutcome(KindCost, err)
		return CostEstimate{}, err
	}
	if saved.ID != "" {
		_ = p.updateRequest(ctx, saved.ID, RequestPatch{
			Status:          RequestSucceeded,
			HTTPStatus:      result.HTTPStatus,
			ResponsePayload: RedactSecrets(result.Body),
			FinishedAt:      time.Now().UTC(),
		})
	}
	var parts *int
	if n, ok := asNumber(obj["cnt"]); ok {
		i := int(n)
		parts = &i
	}
	observeOutcome(KindCost, nil)
	return CostEstimate{
		ProviderCode: ProviderCode,
		CheckType:    checkType,
		PhoneE164:    in.PhoneE164,
		Cost:         asString(obj["cost"]),
		Currency:     p.liveConfig(ctx).Currency,
		Parts:        parts,
		RawResponse:  result.Body,
	}, nil
}

func (p *Provider) submit(ctx context.Context, checkType CheckType, in SubmitInput) (SubmitResult, error) {
	idempotencyKey := SendIdempotencyKey(checkType, in.IdempotencyKey)
	if p.store != nil {
		latest, err := p.store.FindLatestSend(ctx, ProviderCode, in.TenantID, idempotencyKey)
		if err != nil {
			return SubmitResult{}, err
		}
		if latest != nil && latest.Status == RequestSucceeded {
			return p.reuseSucceeded(checkType, in, *latest, in.CorrelationID), nil
		}
		if latest != nil && latest.Status == RequestPending {
			return SubmitResult{}, &Error{
				ProviderCode:  ProviderCode,
				Kind:          KindConflict,
				Message:       "SMSC send already in flight for key " + idempotencyKey,
				Retryable:     true,
				CorrelationID: in.CorrelationID,
			}
		}
	}

	phone := ToPhoneDigits(in.PhoneE164)
	clientID := ClientIDFromKey(idempotencyKey)
	flags := checkTypeFlags(checkType)
	payload := map[string]any{
		"path":           "/sys/send.php",
		"phones":         phone,
		"id":             clientID,
		"checkType":      checkType,
		"jobItemId":      in.JobItemID,
		"idempotencyKey": idempotencyKey,
	}
	for k, v := range flags {
		payload[k] = v
	}
	requestPayload := RedactSecrets(payload)

	saved, err := p.saveRequest(ctx, RequestRecord{
		TenantID:       in.TenantID,
		JobItemID:      in.JobItemID,
		ProviderCode:   ProviderCode,
		Kind:           KindSend,
		Status:         RequestPending,
		RequestPayload: requestPayload,
		IdempotencyKey: idempotencyKey,
		StartedAt:      time.Now().UTC(),
	})
	if err != nil {
		var conflict *IdempotencyConflictError
		if errors.As(err, &conflict) {
			return SubmitResult{}, &Error{
				ProviderCode:  ProviderCode,
				Kind:          KindConflict,
				Message:       "SMSC send already in flight for key " + idempotencyKey,
				Retryable:     true,
				CorrelationID: in.CorrelationID,
				Err:           err,
			}
		}
		return SubmitResult{}, err
	}
	if saved.Deduplicated && p.store != nil {
		existing, err := p.store.FindSucceededSend(ctx, ProviderCode, in.TenantID, idempotencyKey)
		if err != nil {
			return SubmitResult{}, err
		}
		if existing != nil {
			return p.reuseSucceeded(checkType, in, *existing, in.CorrelationID), nil
		}
	}

	params := map[string]any{"phones": phone, "id": clientID}
	for k, v := range flags {
		params[k] = v
	}
	result, err := p.http.Request(ctx, "/sys/send.php", params, in.CorrelationID, string(KindSend))
	if err != nil {
		_ = p.failRequest(ctx, saved.ID, err, &mapFail{CheckType: checkType, PhoneE164: in.PhoneE164})
		observeOutcome(KindSend, err)
		return SubmitResult{}, err
	}
	if obj, ok := asObject(result.Body); ok && hasNonEmpty(obj, "error_code") {
		perr := errorFromBody(obj, in.CorrelationID, result.HTTPStatus)
		_ = p.failRequest(ctx, saved.ID, perr, &mapFail{CheckType: checkType, PhoneE164: in.PhoneE164})
		observeOutcome(KindSend, perr)
		return SubmitResult{}, perr
	}
	obj, _ := asObject(result.Body)
	providerMessageID := asString(obj["id"])
	if providerMessageID == "" {
		perr := &Error{
			ProviderCode:  ProviderCode,
			Kind:          KindProvider,
			Message:       "SMSC send response missing id",
			Retryable:     false,
			CorrelationID: in.CorrelationID,
			RawResponse:   result.Body,
		}
		_ = p.failRequest(ctx, saved.ID, perr, &mapFail{CheckType: checkType, PhoneE164: in.PhoneE164})
		observeOutcome(KindSend, perr)
		return SubmitResult{}, perr
	}
	normalized := p.MapResponse(MapResponseInput{
		CheckType:         checkType,
		Raw:               result.Body,
		PhoneE164:         in.PhoneE164,
		ProviderMessageID: providerMessageID,
	})
	if saved.ID != "" {
		cp := normalized
		_ = p.updateRequest(ctx, saved.ID, RequestPatch{
			Status:            RequestSucceeded,
			HTTPStatus:        result.HTTPStatus,
			ProviderMessageID: providerMessageID,
			ResponsePayload:   RedactSecrets(result.Body),
			Normalized:        &cp,
			FinishedAt:        time.Now().UTC(),
		})
	}
	cost := asString(obj["cost"])
	balance := asString(obj["balance"])
	observeOutcome(KindSend, nil)
	return SubmitResult{
		ProviderCode:      ProviderCode,
		CheckType:         checkType,
		ProviderMessageID: providerMessageID,
		Accepted:          true,
		Deduplicated:      false,
		Cost:              cost,
		Balance:           balance,
		Normalized:        normalized,
		RawRequest:        requestPayload,
		RawResponse:       result.Body,
		ProviderRequestID: saved.ID,
	}, nil
}

func (p *Provider) reuseSucceeded(checkType CheckType, in SubmitInput, existing RequestRecord, correlationID string) SubmitResult {
	rawResponse := existing.ResponsePayload
	if rawResponse == nil {
		rawResponse = map[string]any{}
	}
	normalized := p.MapResponse(MapResponseInput{
		CheckType:         checkType,
		Raw:               rawResponse,
		PhoneE164:         in.PhoneE164,
		ProviderMessageID: existing.ProviderMessageID,
	})
	p.log.Info("smsc.submit.deduplicated",
		"checkType", checkType,
		"jobItemId", in.JobItemID,
		"providerMessageId", existing.ProviderMessageID,
		"correlationId", correlationID,
	)
	balance := ""
	if obj, ok := asObject(rawResponse); ok {
		balance = asString(obj["balance"])
	}
	return SubmitResult{
		ProviderCode:      ProviderCode,
		CheckType:         checkType,
		ProviderMessageID: existing.ProviderMessageID,
		Accepted:          true,
		Deduplicated:      true,
		Cost:              normalized.Cost,
		Balance:           balance,
		Normalized:        normalized,
		RawRequest:        existing.RequestPayload,
		RawResponse:       rawResponse,
		ProviderRequestID: existing.ID,
	}
}

type mapFail struct {
	CheckType         CheckType
	PhoneE164         string
	ProviderMessageID string
}

func (p *Provider) failRequest(ctx context.Context, id string, err error, mapping *mapFail) error {
	if id == "" || p.store == nil {
		return nil
	}
	pe := AsError(err)
	var raw any
	if pe != nil && pe.RawResponse != nil {
		raw = RedactSecrets(pe.RawResponse)
	}
	var normalized *NormalizedResult
	if mapping != nil && raw != nil {
		n := p.MapResponse(MapResponseInput{
			CheckType:         mapping.CheckType,
			Raw:               raw,
			PhoneE164:         mapping.PhoneE164,
			ProviderMessageID: mapping.ProviderMessageID,
		})
		normalized = &n
	}
	code := "unknown"
	msg := ""
	httpStatus := 0
	if pe != nil {
		if pe.ProviderErrorCode != nil {
			code = asString(pe.ProviderErrorCode)
		} else {
			code = string(pe.Kind)
		}
		msg = pe.Message
		httpStatus = pe.HTTPStatus
	} else if err != nil {
		msg = err.Error()
	}
	return p.updateRequest(ctx, id, RequestPatch{
		Status:          RequestFailed,
		HTTPStatus:      httpStatus,
		ErrorCode:       code,
		ErrorMessage:    msg,
		ResponsePayload: raw,
		Normalized:      normalized,
		FinishedAt:      time.Now().UTC(),
	})
}

func (p *Provider) saveRequest(ctx context.Context, rec RequestRecord) (SaveResult, error) {
	if p.store == nil {
		return SaveResult{}, nil
	}
	return p.store.SaveRequest(ctx, rec)
}

func (p *Provider) updateRequest(ctx context.Context, id string, patch RequestPatch) error {
	if p.store == nil || id == "" {
		return nil
	}
	writeCtx, cancel := persistContext(ctx)
	defer cancel()
	return p.store.UpdateRequest(writeCtx, id, patch)
}

func (p *Provider) saveCallback(ctx context.Context, rec CallbackRecord) (SaveResult, error) {
	if p.store == nil {
		return SaveResult{}, nil
	}
	writeCtx, cancel := persistContext(ctx)
	defer cancel()
	return p.store.SaveCallback(writeCtx, rec)
}
