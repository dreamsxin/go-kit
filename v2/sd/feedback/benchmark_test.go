package feedback_test

import (
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
)

// The benchmarks here cover the two things a measurement-driven selection does
// per request: record one call's outcome, and read every candidate's stats to
// choose among them.

func benchInstances(count int) []sd.Instance {
	instances := make([]sd.Instance, count)
	for i := range instances {
		instances[i] = sd.Instance{Address: "10.0.0." + string(rune('1'+i%9)) + ":8080"}
	}
	return instances
}

// runConcurrent runs work on an exact number of callers, which is what a
// contention measurement needs to be comparable across runs.
func runConcurrent(b *testing.B, callers int, work func(caller, iteration int)) {
	b.Helper()
	perCaller := b.N / callers
	if perCaller == 0 {
		perCaller = 1
	}
	b.ResetTimer()
	var wait sync.WaitGroup
	for caller := 0; caller < callers; caller++ {
		wait.Add(1)
		go func(caller int) {
			defer wait.Done()
			for i := 0; i < perCaller; i++ {
				work(caller, i)
			}
		}(caller)
	}
	wait.Wait()
}

func BenchmarkTableTrackAndComplete(b *testing.B) {
	table := feedback.NewTable()
	instance := benchInstances(1)[0]
	outcome := sd.Outcome{Latency: time.Millisecond}
	b.ReportAllocs()
	for b.Loop() {
		table.Track(instance)(outcome)
	}
}

func BenchmarkTableTrackAndCompleteConcurrent(b *testing.B) {
	for _, callers := range []int{8, 64} {
		b.Run(callerLabel(callers), func(b *testing.B) {
			table := feedback.NewTable()
			instances := benchInstances(9)
			outcome := sd.Outcome{Latency: time.Millisecond}
			b.ReportAllocs()
			runConcurrent(b, callers, func(caller, iteration int) {
				table.Track(instances[(caller+iteration)%len(instances)])(outcome)
			})
		})
	}
}

// BenchmarkTableLoadAcrossCandidates is the read half of one selection: the
// strategy asks for every candidate's load before it picks.
func BenchmarkTableLoadAcrossCandidates(b *testing.B) {
	table := feedback.NewTable()
	instances := benchInstances(9)
	for _, instance := range instances {
		table.Observe(instance, sd.Outcome{Latency: time.Millisecond})
	}
	load := table.Load()
	b.ReportAllocs()
	for b.Loop() {
		for _, instance := range instances {
			_ = load(instance)
		}
	}
}

func BenchmarkTableLoadAcrossCandidatesConcurrent(b *testing.B) {
	for _, callers := range []int{8, 64} {
		b.Run(callerLabel(callers), func(b *testing.B) {
			table := feedback.NewTable()
			instances := benchInstances(9)
			for _, instance := range instances {
				table.Observe(instance, sd.Outcome{Latency: time.Millisecond})
			}
			load := table.Load()
			b.ReportAllocs()
			runConcurrent(b, callers, func(int, int) {
				for _, instance := range instances {
					_ = load(instance)
				}
			})
		})
	}
}

// BenchmarkTableSelectionRoundTrip is the whole per-request cost of a
// measurement-driven selection: read every candidate, then record the outcome.
func BenchmarkTableSelectionRoundTrip(b *testing.B) {
	for _, callers := range []int{8, 64} {
		b.Run(callerLabel(callers), func(b *testing.B) {
			table := feedback.NewTable()
			instances := benchInstances(9)
			load := table.Load()
			outcome := sd.Outcome{Latency: time.Millisecond}
			b.ReportAllocs()
			runConcurrent(b, callers, func(caller, iteration int) {
				for _, instance := range instances {
					_ = load(instance)
				}
				table.Track(instances[(caller+iteration)%len(instances)])(outcome)
			})
		})
	}
}

func callerLabel(callers int) string {
	if callers == 8 {
		return "eight-callers"
	}
	return "sixty-four-callers"
}
