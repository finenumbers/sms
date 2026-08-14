package admin

import (
	"testing"
	"time"
)

func TestParseLogsWindowDefault(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	from, to, err := parseLogsWindow("", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if to != now || now.Sub(from) != time.Hour {
		t.Fatalf("from=%s to=%s", from, to)
	}
}

func TestParseLogsWindowMaxSpan(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	_, _, err := parseLogsWindow(now.Add(-25*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now)
	if err == nil {
		t.Fatal("expected span error")
	}
}

func TestLikeContainsStripsWildcards(t *testing.T) {
	if likeContains(`%_foo\`) != "%foo%" {
		t.Fatalf("%q", likeContains(`%_foo\`))
	}
	if likeContains("   ") != "" {
		t.Fatal("empty")
	}
}
