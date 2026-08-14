package kit

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingLifecycle struct {
	name        string
	events      *[]string
	errors      chan error
	startErr    error
	shutdownErr error
}

func (c *recordingLifecycle) Start() error {
	*c.events = append(*c.events, "start "+c.name)
	return c.startErr
}

func (c *recordingLifecycle) Errors() <-chan error {
	return c.errors
}

func (c *recordingLifecycle) Shutdown(context.Context) error {
	*c.events = append(*c.events, "stop "+c.name)
	return c.shutdownErr
}

func TestLifecycleComponentsStartAndStopInDependencyOrder(t *testing.T) {
	var events []string
	first := &recordingLifecycle{name: "first", events: &events}
	second := &recordingLifecycle{name: "second", events: &events}
	service := MustNew("127.0.0.1:0", WithLifecycle(first, second))

	if err := service.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	want := []string{"start first", "start second", "stop second", "stop first"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

func TestLifecycleStartupFailureStopsPreviouslyStartedComponents(t *testing.T) {
	var events []string
	first := &recordingLifecycle{name: "first", events: &events}
	second := &recordingLifecycle{name: "second", events: &events, startErr: errors.New("failed")}
	service := MustNew("127.0.0.1:0", WithLifecycle(first, second))

	if err := service.Start(); err == nil {
		t.Fatal("expected startup error")
	}
	want := []string{"start first", "start second", "stop first"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
	if err := service.Start(); err == nil {
		t.Fatal("expected service with cleaned-up components to reject restart")
	}
}

func TestLifecycleAsynchronousErrorStopsRun(t *testing.T) {
	var events []string
	errorsChannel := make(chan error, 1)
	component := &recordingLifecycle{name: "worker", events: &events, errors: errorsChannel}
	service := MustNew("127.0.0.1:0", WithLifecycle(component), WithShutdownTimeout(time.Second))
	done := make(chan error, 1)
	go func() {
		done <- service.Run(context.Background())
	}()

	errorsChannel <- errors.New("serve failed")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "serve failed") {
			t.Fatalf("Run error = %v, want component failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after component failure")
	}
}

func TestWithLifecycleRejectsTypedNil(t *testing.T) {
	var component *recordingLifecycle
	if _, err := New(":0", WithLifecycle(component)); err == nil {
		t.Fatal("expected typed nil lifecycle error")
	}
}
