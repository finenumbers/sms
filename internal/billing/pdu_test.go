package billing

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestSegmentCountGSM7(t *testing.T) {
	if n := SegmentCount("hello"); n != 1 {
		t.Fatalf("got %d", n)
	}
	long := make([]byte, 160)
	for i := range long {
		long[i] = 'a'
	}
	if n := SegmentCount(string(long)); n != 1 {
		t.Fatalf("160 gsm got %d", n)
	}
	if n := SegmentCount(string(long)+"a"); n != 2 {
		t.Fatalf("161 gsm got %d", n)
	}
	if n := SegmentCount("€"); n != 1 {
		t.Fatalf("euro escape got %d", n)
	}
}

func TestSegmentCountUCS2(t *testing.T) {
	if n := SegmentCount("Привет"); n != 1 {
		t.Fatalf("cyrillic got %d", n)
	}
	runes := make([]rune, 70)
	for i := range runes {
		runes[i] = 'Я'
	}
	if n := SegmentCount(string(runes)); n != 1 {
		t.Fatalf("70 ucs2 got %d", n)
	}
	if n := SegmentCount(string(runes)+"Я"); n != 2 {
		t.Fatalf("71 ucs2 got %d", n)
	}
	if n := SegmentCount("👍"); n != 1 {
		t.Fatalf("emoji got %d", n)
	}
}

func TestApplyHoldDebitRelease(t *testing.T) {
	ten := decimal.RequireFromString("10")
	three := decimal.RequireFromString("3")
	zero := decimal.Zero
	avail, held, err := applyHold(ten, zero, three)
	if err != nil || !avail.Equal(decimal.RequireFromString("7")) || !held.Equal(three) {
		t.Fatalf("hold %s %s %v", avail, held, err)
	}
	if _, _, err := applyHold(decimal.RequireFromString("2"), zero, three); err == nil {
		t.Fatal("expected insufficient")
	}
	avail, held, err = applyDebitFromHold(decimal.RequireFromString("7"), three, three)
	if err != nil || !avail.Equal(decimal.RequireFromString("7")) || !held.IsZero() {
		t.Fatalf("debit %s %s %v", avail, held, err)
	}
	avail, held, err = applyRelease(decimal.RequireFromString("7"), three, three)
	if err != nil || !avail.Equal(ten) || !held.IsZero() {
		t.Fatalf("release %s %s %v", avail, held, err)
	}
}
