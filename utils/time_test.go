package utils_test

import (
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/utils"
)

func TestExponential_Doubles(t *testing.T) {
	d := 10 * time.Millisecond
	next := utils.Exponential(d)
	// with jitter [0.5, 1.5], result should be in [10ms, 30ms]
	if next < 10*time.Millisecond || next > 30*time.Millisecond {
		t.Errorf("expected result in [10ms, 30ms], got %v", next)
	}
}

func TestExponential_CapsAtOneMinute(t *testing.T) {
	d := 30 * time.Second
	for i := 0; i < 10; i++ {
		d = utils.Exponential(d)
		if d > time.Minute {
			t.Errorf("Exponential exceeded 1 minute cap: %v", d)
		}
	}
}

func TestExponential_NeverZero(t *testing.T) {
	d := 1 * time.Nanosecond
	for i := 0; i < 20; i++ {
		d = utils.Exponential(d)
		if d <= 0 {
			t.Errorf("Exponential returned non-positive duration: %v", d)
		}
	}
}
