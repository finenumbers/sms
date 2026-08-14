package smsc

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"strings"
)

const probePhone = "+79000000000"

type ConnectivityProbe struct {
	Configured               bool   `json:"configured"`
	CallbackSecretConfigured bool   `json:"callback_secret_configured"`
	BalanceOK                bool   `json:"balance_ok"`
	SignatureOK              bool   `json:"signature_ok"`
	Balance                  string `json:"balance,omitempty"`
	Currency                 string `json:"currency,omitempty"`
	BalanceError             string `json:"balance_error,omitempty"`
}

func (p *Provider) ProbeLocalSignature() bool {
	secret := ""
	if p != nil {
		secret = strings.TrimSpace(p.liveConfig(context.Background()).CallbackSecret)
	}
	if secret == "" {
		return false
	}
	phone := ToPhoneDigits(probePhone)
	payload := map[string]any{"id": "probe", "phone": phone, "status": "1"}
	base := "probe:" + phone + ":1:" + secret
	sum := md5.Sum([]byte(base))
	v := VerifyCallbackSignature(VerifyInput{
		Payload: payload,
		Secret:  secret,
		Signatures: CallbackSignatures{
			MD5: hex.EncodeToString(sum[:]),
		},
	})
	return v != nil && *v
}

func (p *Provider) ProbeConnectivity(ctx context.Context) ConnectivityProbe {
	out := ConnectivityProbe{
		Configured:               p.Configured(),
		CallbackSecretConfigured: p.CallbackSecretConfigured(),
		SignatureOK:              p.ProbeLocalSignature(),
	}
	if !p.Configured() {
		out.BalanceError = "smsc credentials are not configured"
		return out
	}
	bal, err := p.Balance(ctx, "admin-connectivity")
	if err != nil {
		out.BalanceError = err.Error()
		return out
	}
	out.BalanceOK = true
	out.Balance = bal.Balance
	out.Currency = bal.Currency
	return out
}

func (p *Provider) ProbeEstimateCost(ctx context.Context, checkType CheckType, phone string) (CostEstimate, error) {
	if !p.Configured() {
		return CostEstimate{}, errors.New("smsc credentials are not configured")
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		phone = probePhone
	}
	in := SubmitInput{PhoneE164: phone, CorrelationID: "admin-estimate-cost"}
	switch NormalizeCheckType(checkType) {
	case CheckPing:
		return p.EstimatePingCost(ctx, in)
	default:
		return p.EstimateHLRCost(ctx, in)
	}
}
