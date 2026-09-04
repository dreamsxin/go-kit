package kit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// Health checks run in their own goroutines, so an unrecovered panic in one used
// to take the process down — from an unauthenticated probe.
func TestReadinessCheckPanicDoesNotKillTheProcess(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithReadinessCheck("db", func(context.Context) error {
			panic("driver went away")
		}),
	)

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "check panicked") {
		t.Fatalf("body should name the panic, got %q", body)
	}
	// The panic value may carry internals; the probe response must not.
	if strings.Contains(body, "driver went away") {
		t.Fatalf("body leaked the panic value: %q", body)
	}
}

// A panicking check must not make the healthy ones unreadable.
func TestReadinessReportsOtherChecksAroundAPanic(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithReadinessCheck("bad", func(context.Context) error { panic("boom") }),
		kit.WithReadinessCheck("good", func(context.Context) error { return nil }),
	)

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	var payload struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body %s: %v", recorder.Body.String(), err)
	}
	statuses := map[string]string{}
	for _, check := range payload.Checks {
		statuses[check.Name] = check.Status
	}
	if statuses["good"] != "ok" {
		t.Fatalf("good check status = %q, want ok (%v)", statuses["good"], statuses)
	}
	if statuses["bad"] != "error" {
		t.Fatalf("bad check status = %q, want error (%v)", statuses["bad"], statuses)
	}
}
