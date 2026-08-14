package webhooks

import "time"

func RetryDelay(attemptCount int32) time.Duration {
	if attemptCount < 1 {
		attemptCount = 1
	}
	exp := attemptCount - 1
	if exp > 8 {
		exp = 8
	}
	return time.Duration(30*(1<<exp)) * time.Second
}

func NextAttemptAt(attemptCount, maxAttempts int32, now time.Time) *time.Time {
	if attemptCount >= maxAttempts {
		return nil
	}
	at := now.Add(RetryDelay(attemptCount))
	return &at
}
