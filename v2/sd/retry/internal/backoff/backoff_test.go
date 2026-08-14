package backoff

import (
	"testing"
	"time"
)

func TestNextBoundsAndCap(t *testing.T) {
	next := Next(10 * time.Millisecond)
	if next < 10*time.Millisecond || next > 30*time.Millisecond {
		t.Fatalf("Next returned %v, want [10ms, 30ms]", next)
	}
	if capped := Next(time.Minute); capped != time.Minute {
		t.Fatalf("Next cap = %v, want 1m", capped)
	}
}
