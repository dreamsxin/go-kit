package utils

import (
	"math/rand"
	"time"
)

// Exponential doubles d, applies ±50 % jitter, and caps the result at one
// minute.  It is used by RetryWithCallback to implement exponential backoff
// between retry attempts.
func Exponential(d time.Duration) time.Duration {
	d *= 2
	d = time.Duration(int64(float64(d.Nanoseconds()) * (rand.Float64() + 0.5)))
	if d > time.Minute {
		d = time.Minute
	}
	return d

}
