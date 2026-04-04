package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
)

// ── Errors ────────────────────────────────────────────────────────────────────

// ErrHystrixCircuitOpen is returned when the Hystrix circuit is open and
// the request is rejected without calling the wrapped endpoint.
var ErrHystrixCircuitOpen = errors.New("hystrix: circuit open")

// ErrHystrixTimeout is returned when the wrapped endpoint exceeds the
// configured timeout.
var ErrHystrixTimeout = errors.New("hystrix: timeout")

// ErrHystrixMaxConcurrency is returned when the number of in-flight requests
// exceeds MaxConcurrentRequests.
var ErrHystrixMaxConcurrency = errors.New("hystrix: max concurrency reached")

// ── Configuration ─────────────────────────────────────────────────────────────

// HystrixConfig holds the configuration for a single Hystrix command.
// All fields have sensible defaults; zero values are replaced by defaults
// when the command is first used.
type HystrixConfig struct {
	// Timeout is the maximum duration for a single request (default: 1s).
	Timeout time.Duration

	// MaxConcurrentRequests is the maximum number of in-flight requests
	// allowed at any time (default: 10).
	MaxConcurrentRequests int

	// RequestVolumeThreshold is the minimum number of requests in the
	// rolling window before the error rate is evaluated (default: 20).
	RequestVolumeThreshold int

	// SleepWindow is how long to wait after the circuit opens before
	// allowing a single probe request (default: 5s).
	SleepWindow time.Duration

	// ErrorPercentThreshold is the error rate (0–100) above which the
	// circuit opens (default: 50).
	ErrorPercentThreshold int
}

func (c HystrixConfig) withDefaults() HystrixConfig {
	out := c
	if out.Timeout <= 0 {
		out.Timeout = time.Second
	}
	if out.MaxConcurrentRequests <= 0 {
		out.MaxConcurrentRequests = 10
	}
	if out.RequestVolumeThreshold <= 0 {
		out.RequestVolumeThreshold = 20
	}
	if out.SleepWindow <= 0 {
		out.SleepWindow = 5 * time.Second
	}
	if out.ErrorPercentThreshold <= 0 {
		out.ErrorPercentThreshold = 50
	}
	return out
}

// ── Command registry ──────────────────────────────────────────────────────────

var (
	commandsMu sync.RWMutex
	commands   = map[string]*hystrixCommand{}
)

// HystrixConfigureCommand registers or updates the configuration for a named
// command.  Call this before using Hystrix() with the same name.
func HystrixConfigureCommand(name string, cfg HystrixConfig) {
	commandsMu.Lock()
	defer commandsMu.Unlock()
	normalized := cfg.withDefaults()
	if existing, ok := commands[name]; ok {
		existing.cfg = normalized
		return
	}
	commands[name] = newHystrixCommand(name, normalized)
}

func getOrCreateCommand(name string) *hystrixCommand {
	commandsMu.RLock()
	cmd, ok := commands[name]
	commandsMu.RUnlock()
	if ok {
		return cmd
	}
	commandsMu.Lock()
	defer commandsMu.Unlock()
	if cmd, ok = commands[name]; ok {
		return cmd
	}
	cmd = newHystrixCommand(name, HystrixConfig{}.withDefaults())
	commands[name] = cmd
	return cmd
}

// ── Circuit state ─────────────────────────────────────────────────────────────

type circuitState int32

const (
	stateClosed   circuitState = 0
	stateOpen     circuitState = 1
	stateHalfOpen circuitState = 2
)

// ── Rolling window counter ────────────────────────────────────────────────────

// bucket holds counts for a single 1-second slot.
type bucket struct {
	total   int64
	errors  int64
	ts      int64 // unix second
}

// rollingWindow is a 10-bucket (10-second) sliding window.
type rollingWindow struct {
	mu      sync.Mutex
	buckets [10]bucket
	pos     int
}

func (w *rollingWindow) record(success bool) {
	now := time.Now().Unix()
	w.mu.Lock()
	defer w.mu.Unlock()

	// Advance to the current second, clearing stale buckets.
	for w.buckets[w.pos].ts != now {
		w.pos = (w.pos + 1) % 10
		w.buckets[w.pos] = bucket{ts: now}
	}
	w.buckets[w.pos].total++
	if !success {
		w.buckets[w.pos].errors++
	}
}

func (w *rollingWindow) counts() (total, errs int64) {
	now := time.Now().Unix()
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := 0; i < 10; i++ {
		b := &w.buckets[i]
		if now-b.ts < 10 {
			total += b.total
			errs += b.errors
		}
	}
	return
}

// ── hystrixCommand ────────────────────────────────────────────────────────────

type hystrixCommand struct {
	name      string
	cfg       HystrixConfig
	state     int32 // circuitState
	openedAt  int64 // unix nano, set when circuit opens
	inflight  int64
	window    rollingWindow
}

func newHystrixCommand(name string, cfg HystrixConfig) *hystrixCommand {
	return &hystrixCommand{name: name, cfg: cfg}
}

func (c *hystrixCommand) allowRequest() bool {
	state := circuitState(atomic.LoadInt32(&c.state))
	switch state {
	case stateClosed:
		return true
	case stateOpen:
		// Check if sleep window has elapsed → try half-open
		openedAt := atomic.LoadInt64(&c.openedAt)
		if time.Now().UnixNano()-openedAt >= c.cfg.SleepWindow.Nanoseconds() {
			// Transition to half-open: allow exactly one probe
			if atomic.CompareAndSwapInt32(&c.state, int32(stateOpen), int32(stateHalfOpen)) {
				return true
			}
		}
		return false
	case stateHalfOpen:
		// Only one probe at a time; subsequent requests are rejected
		return false
	}
	return true
}

func (c *hystrixCommand) recordResult(success bool) {
	c.window.record(success)

	state := circuitState(atomic.LoadInt32(&c.state))
	if state == stateHalfOpen {
		if success {
			atomic.StoreInt32(&c.state, int32(stateClosed))
		} else {
			atomic.StoreInt64(&c.openedAt, time.Now().UnixNano())
			atomic.StoreInt32(&c.state, int32(stateOpen))
		}
		return
	}

	if state == stateClosed {
		total, errs := c.window.counts()
		if total >= int64(c.cfg.RequestVolumeThreshold) {
			errPct := float64(errs) / float64(total) * 100
			if errPct >= float64(c.cfg.ErrorPercentThreshold) {
				atomic.StoreInt64(&c.openedAt, time.Now().UnixNano())
				atomic.StoreInt32(&c.state, int32(stateOpen))
			}
		}
	}
}

// ── Middleware ────────────────────────────────────────────────────────────────

// Hystrix returns an endpoint.Middleware that implements the Hystrix circuit
// breaker pattern.  It is a drop-in replacement for the afex/hystrix-go
// package with no external dependency.
//
// Configure the command before use:
//
//	circuitbreaker.HystrixConfigureCommand("my-endpoint", circuitbreaker.HystrixConfig{
//	    Timeout:                time.Second,
//	    MaxConcurrentRequests:  100,
//	    RequestVolumeThreshold: 20,
//	    SleepWindow:            5 * time.Second,
//	    ErrorPercentThreshold:  50,
//	})
//	ep = circuitbreaker.Hystrix("my-endpoint")(ep)
func Hystrix(commandName string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			cmd := getOrCreateCommand(commandName)

			// Concurrency check
			cur := atomic.AddInt64(&cmd.inflight, 1)
			defer atomic.AddInt64(&cmd.inflight, -1)
			if cur > int64(cmd.cfg.MaxConcurrentRequests) {
				cmd.recordResult(false)
				return nil, ErrHystrixMaxConcurrency
			}

			// Circuit check
			if !cmd.allowRequest() {
				return nil, ErrHystrixCircuitOpen
			}

			// Execute with timeout
			type result struct {
				resp interface{}
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				resp, err := next(ctx, request)
				ch <- result{resp, err}
			}()

			select {
			case r := <-ch:
				cmd.recordResult(r.err == nil)
				return r.resp, r.err
			case <-time.After(cmd.cfg.Timeout):
				cmd.recordResult(false)
				return nil, ErrHystrixTimeout
			case <-ctx.Done():
				cmd.recordResult(false)
				return nil, ctx.Err()
			}
		}
	}
}
