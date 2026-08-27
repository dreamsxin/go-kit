package kit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

type namedComponent struct {
	recordingLifecycle
}

func (c *namedComponent) Name() string { return c.name }

func TestLifecycleAsyncErrorReportsComponentName(t *testing.T) {
	var events []string
	errorsChannel := make(chan error, 1)
	component := &namedComponent{recordingLifecycle{name: "worker", events: &events, errors: errorsChannel}}
	service := MustNew("127.0.0.1:0", WithLifecycle(component), WithShutdownTimeout(time.Second))
	done := make(chan error, 1)
	go func() {
		done <- service.Run(context.Background())
	}()

	errorsChannel <- errors.New("serve failed")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "lifecycle component worker") {
			t.Fatalf("Run error = %v, want named component failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after component failure")
	}
}

func TestLifecycleWatchConsumesMultipleAsyncErrors(t *testing.T) {
	var events []string
	errorsChannel := make(chan error, 2)
	component := &recordingLifecycle{name: "worker", events: &events, errors: errorsChannel}
	service := MustNew("127.0.0.1:0", WithLifecycle(component))
	if err := service.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer service.Shutdown(context.Background()) //nolint:errcheck

	errorsChannel <- errors.New("first failure")
	errorsChannel <- errors.New("second failure")

	for _, want := range []string{"first failure", "second failure"} {
		select {
		case err := <-service.Errors():
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("reported error = %v, want %q", err, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("did not observe %q", want)
		}
	}
}

type readinessComponent struct {
	namedComponent
	ready chan struct{}
}

func (c *readinessComponent) Ready(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestReadinessProviderBridgedToReadyz(t *testing.T) {
	var events []string
	component := &readinessComponent{
		namedComponent: namedComponent{recordingLifecycle{name: "warmer", events: &events}},
		ready:          make(chan struct{}),
	}
	service := MustNew("127.0.0.1:0", WithLifecycle(component))

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz before warmup = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(recorder.Body.String(), "lifecycle:warmer") {
		t.Fatalf("readyz body = %s, want lifecycle:warmer check", recorder.Body.String())
	}

	close(component.ready)
	recorder = httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("readyz after warmup = %d, want %d", recorder.Code, http.StatusOK)
	}
}
