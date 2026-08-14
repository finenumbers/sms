package messaging

import (
	"strings"

	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/runexis"
)

func MatchStatistic(from, to, text string, rows []runexis.StatisticRow, used map[string]struct{}) (runexis.StatisticRow, bool) {
	wantFrom := msisdn.Canonical(from)
	wantTo := msisdn.Canonical(to)
	wantText := strings.TrimSpace(text)

	var exact []runexis.StatisticRow
	var empty []runexis.StatisticRow
	for _, row := range rows {
		if row.Incoming {
			continue
		}
		if msisdn.Canonical(row.SenderNumber) != wantFrom {
			continue
		}
		if msisdn.Canonical(row.ReceiverNumber) != wantTo {
			continue
		}
		if row.SMSID != "" {
			if _, ok := used[row.SMSID]; ok {
				continue
			}
		}
		msg := strings.TrimSpace(row.Message)
		if msg != "" {
			if msg != wantText {
				continue
			}
			exact = append(exact, row)
			continue
		}
		empty = append(empty, row)
	}
	if len(exact) > 0 {
		return exact[0], true
	}
	if len(empty) == 1 && empty[0].SMSID != "" {
		return empty[0], true
	}
	return runexis.StatisticRow{}, false
}
