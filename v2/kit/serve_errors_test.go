package kit_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// TestRunJoinsEveryAsyncComponentFailure proves the retained record, not the
// wake-up channel, is what Run reports. A burst of asynchronous failures wider
// than the Errors buffer — five of them against a two-slot buffer — used to
// lose everything the channel could not take; the stable contract
// kit.serve-errors-retained (host.go) says every one of them reaches the
// result.
func TestRunJoinsEveryAsyncComponentFailure(t *testing.T) {
	var events []string
	component := &recordingLifecycle{name: "worker", events: &events, errors: make(chan error, 5)}
	host := kit.MustNewHost(
		kit.WithLifecycle(component),
		kit.WithShutdownTimeout(time.Second),
		kit.WithDrainDelay(0),
	)

	// Fill the whole burst before Run starts: a channel send is a scheduling
	// point, so starting Run first lets it announce the stop while the test
	// goroutine is still filling the buffer, and an error arriving after the
	// announcement is outside the retained-record promise. Pre-filling makes
	// every failure a pre-announcement one the watcher must drain, which is
	// the property under test.
	for i := 0; i < 5; i++ {
		component.errors <- fmt.Errorf("failure %d", i)
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned no error for a component that failed")
		}
		for i := 0; i < 5; i++ {
			if !strings.Contains(err.Error(), fmt.Sprintf("failure %d", i)) {
				t.Errorf("Run's error lost failure %d: %v", i, err)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

// TestRunRetainsAWideBurst widens the window the watcher used to lose: Run
// wakes on the first failure and announces the stop while dozens of
// pre-shutdown failures are still buffered in the component channel. The
// retained record must name every one of them; the watcher drains the buffer
// on the stop signal before leaving. Pre-fix the watcher's select took the
// stop and exited, so most of the burst was missing.
func TestRunRetainsAWideBurst(t *testing.T) {
	const total = 50
	var events []string
	component := &recordingLifecycle{name: "worker", events: &events, errors: make(chan error, total)}
	host := kit.MustNewHost(
		kit.WithLifecycle(component),
		kit.WithShutdownTimeout(time.Second),
		kit.WithDrainDelay(0),
	)

	// Pre-fill before Run starts (see TestRunJoinsEveryAsyncComponentFailure):
	// all fifty are then pre-announcement errors the watcher must drain when
	// the stop and the buffered errors are both ready.
	for i := 0; i < total; i++ {
		component.errors <- fmt.Errorf("failure %d", i)
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned no error for a component that failed")
		}
		for i := 0; i < total; i++ {
			if !strings.Contains(err.Error(), fmt.Sprintf("failure %d", i)) {
				t.Errorf("Run's error lost failure %d", i)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

// TestRunJoinsServeErrorsArrivingDuringDrain covers the window the wake-up
// channel never protected: Run has already been woken by the first failure and
// is announcing the stop, so nothing selects on Errors any more. The watchers
// are still forwarding, and the retained set — the thing Run joins — keeps
// taking the reports.
func TestRunJoinsServeErrorsArrivingDuringDrain(t *testing.T) {
	var events []string
	component := &recordingLifecycle{name: "worker", events: &events, errors: make(chan error, 1)}
	host := kit.MustNewHost(
		kit.WithLifecycle(component),
		kit.WithShutdownTimeout(time.Second),
		kit.WithDrainDelay(500*time.Millisecond),
	)

	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()
	component.errors <- errors.New("first failure")
	// Give Run a moment to wake and enter the drain window; the delay is
	// wide so a slow scheduler still lands the send inside it. If Run has
	// not woken yet, the report is still retained and the assertion below
	// holds regardless.
	time.Sleep(30 * time.Millisecond)
	component.errors <- errors.New("failure during drain")

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned no error for a component that failed")
		}
		for _, want := range []string{"first failure", "failure during drain"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Run's error lost %q: %v", want, err)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}
