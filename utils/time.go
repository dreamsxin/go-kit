package utils

import (
	"math/rand"
	"time"
)

// 指数增加时间
func Exponential(d time.Duration) time.Duration {
	d *= 2
	d = time.Duration(int64(float64(d.Nanoseconds()) * (rand.Float64() + 0.5)))
	if d > time.Minute {
		d = time.Minute
	}
	return d

}
