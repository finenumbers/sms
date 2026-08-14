package smsc

import (
	"strings"

	"finenumbers/sms/internal/metrics"
)

func observeOutcome(kind RequestKind, err error) {
	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	metrics.ObserveSMSCRequest(strings.ToLower(string(kind)), status)
	if pe := AsError(err); pe != nil {
		if n, ok := asInt(pe.ProviderErrorCode); ok && n == 9 {
			metrics.ObserveSMSCErrorCode9()
		}
	}
}
