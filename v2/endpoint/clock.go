package endpoint

import (
	"sync"
	"time"
)

// Clock is the seam that decides what time it is and how long a wait takes.
//
// It lives here because this is the stdlib-only layer every other layer is
// allowed to import, the same reason Recorder and RateLimiter live here. A nil
// Clock anywhere in this framework means the wall clock: a service that does not
// care about time as an input writes nothing, and nothing about its behaviour
// changes.
//
// It exists for the deployment that has to prove its own timing. A backoff
// schedule, a token expiry and a session TTL are all decisions a service makes
// from the clock, and a test that cannot move the clock has to sleep — which
// makes the test slow when it passes and flaky when the machine is loaded.
//
// Stable: endpoint.clock-nil-is-wall-clock — a nil Clock means the wall clock, so a service that ignores this seam behaves exactly as before.
// Covered by: TestNilClockIsTheWallClock
type Clock interface {
	// Now reports the current time.
	Now() time.Time
	// NewTimer starts one wait of length d. A non-positive d is due
	// immediately.
	NewTimer(d time.Duration) Timer
}

// Timer is one scheduled wait. It is an interface rather than *time.Timer
// because a controllable clock has to be able to satisfy it.
type Timer interface {
	// C receives once when the wait is due.
	C() <-chan time.Time
	// Stop cancels the wait, reporting whether it was still pending.
	Stop() bool
}

// SystemClock returns the wall clock — what every Clock seam falls back to when
// it is nil. Ask for it explicitly when a struct field must never be nil:
//
//	if settings.clock == nil {
//	    settings.clock = endpoint.SystemClock()
//	}
func SystemClock() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTimer(d time.Duration) Timer {
	if d <= 0 {
		// time.NewTimer panics on nothing here, but a non-positive duration
		// still has to mean "already due" so both clocks agree.
		d = time.Nanosecond
	}
	return systemTimer{timer: time.NewTimer(d)}
}

type systemTimer struct {
	timer *time.Timer
}

func (t systemTimer) C() <-chan time.Time { return t.timer.C }

func (t systemTimer) Stop() bool { return t.timer.Stop() }

// ManualClock is a Clock a test moves by hand. It is exported, and shipped
// beside the contract rather than in a test-only package, because a seam nobody
// can reach is not a seam: the point is that an application can test its own
// timing with the same clock this framework's components accept.
//
//	clock := endpoint.NewManualClock(time.Unix(0, 0))
//	ep := endpoint.NewBuilder(call).
//	    WithRetry(3, endpoint.WithRetryClock(clock)).
//	    Build()
//	go ep(ctx, request)
//	clock.Advance(50 * time.Millisecond) // the first backoff step elapses
//
// It is safe for concurrent use: the code under test reads the clock from its
// own goroutines while the test advances it.
//
// Stable: endpoint.manual-clock-advance-fires-due-timers — advancing a ManualClock past a timer's deadline delivers on that timer, so a waiting goroutine proceeds without real time passing.
// Covered by: TestManualClockAdvanceFiresDueTimers, TestManualClockAdvanceWakesAWaitingGoroutine
type ManualClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*manualTimer
}

// NewManualClock returns a clock reading at, which advances only when Advance or
// Set is called.
func NewManualClock(at time.Time) *ManualClock {
	return &ManualClock{now: at}
}

// Now reports the time this clock was last set to.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer registers a wait due d after the current time. A non-positive d is
// due immediately, without waiting for an Advance.
func (c *ManualClock) NewTimer(d time.Duration) Timer {
	timer := &manualTimer{channel: make(chan time.Time, 1)}
	c.mu.Lock()
	defer c.mu.Unlock()
	timer.due = c.now.Add(d)
	if d <= 0 {
		timer.fire(c.now)
		return timer
	}
	c.pending = append(c.pending, timer)
	return timer
}

// Advance moves the clock forward by d and delivers on every timer that becomes
// due. A non-positive d does nothing.
func (c *ManualClock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.fireDueLocked()
	c.mu.Unlock()
}

// Set moves the clock to at, delivering on every timer that becomes due.
// Setting the clock backwards does not un-fire anything.
func (c *ManualClock) Set(at time.Time) {
	c.mu.Lock()
	c.now = at
	c.fireDueLocked()
	c.mu.Unlock()
}

// Pending reports how many timers are still waiting. A test uses it to know the
// code under test has reached its wait before advancing the clock.
func (c *ManualClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

func (c *ManualClock) fireDueLocked() {
	remaining := c.pending[:0]
	for _, timer := range c.pending {
		if timer.due.After(c.now) {
			remaining = append(remaining, timer)
			continue
		}
		timer.fire(c.now)
	}
	c.pending = remaining
}

type manualTimer struct {
	mu      sync.Mutex
	channel chan time.Time
	due     time.Time
	done    bool
}

func (t *manualTimer) C() <-chan time.Time { return t.channel }

func (t *manualTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	stopped := !t.done
	t.done = true
	return stopped
}

func (t *manualTimer) fire(at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return
	}
	t.done = true
	// The channel is buffered, so a clock advanced before the code under test
	// reaches its select still delivers.
	t.channel <- at
}
