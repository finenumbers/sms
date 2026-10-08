package outbox

import "testing"

func TestNextInboxStatisticPage(t *testing.T) {
	more, trunc := nextInboxStatisticPage(20, 50, 1, 20)
	if more || trunc {
		t.Fatalf("short page: more=%v trunc=%v", more, trunc)
	}
	more, trunc = nextInboxStatisticPage(50, 50, 1, 20)
	if !more || trunc {
		t.Fatalf("full first: more=%v trunc=%v", more, trunc)
	}
	more, trunc = nextInboxStatisticPage(50, 50, 20, 20)
	if more || !trunc {
		t.Fatalf("cap: more=%v trunc=%v", more, trunc)
	}
	more, trunc = nextInboxStatisticPage(0, 50, 1, 20)
	if more || trunc {
		t.Fatalf("empty: more=%v trunc=%v", more, trunc)
	}
}
