package webhooks

const (
	EventCheckCompleted = "check.completed"
	EventCheckFailed    = "check.failed"
	EventJobCompleted   = "job.completed"
	AutoDisableAfter    = 20
	UserAgent           = "Finenumbers-Webhooks/1.0"
	APIVersion          = "v1"
)

var AllEvents = []string{EventCheckCompleted, EventCheckFailed, EventJobCompleted}

func KnownEvent(v string) bool {
	switch v {
	case EventCheckCompleted, EventCheckFailed, EventJobCompleted:
		return true
	default:
		return false
	}
}

func EventForItemStatus(status string) string {
	if status == "completed" {
		return EventCheckCompleted
	}
	return EventCheckFailed
}

func Subscribes(events []string, eventType string) bool {
	if len(events) == 0 {
		return KnownEvent(eventType)
	}
	for _, e := range events {
		if e == eventType {
			return true
		}
	}
	return false
}
