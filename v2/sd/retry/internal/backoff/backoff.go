package backoff

import (
	"math/rand"
	"time"
)

const maximum = time.Minute

// Next doubles delay, applies 50-150 percent jitter, and caps the result.
func Next(delay time.Duration) time.Duration {
	delay *= 2
	delay = time.Duration(float64(delay) * (rand.Float64() + 0.5))
	if delay > maximum {
		return maximum
	}
	return delay
}
