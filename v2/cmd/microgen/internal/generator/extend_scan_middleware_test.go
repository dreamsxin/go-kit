package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectGeneratedMiddlewaresSeesCurrentMetricsString pins the extend-mode
// detector against the template's current middleware text. The detector used
// to match the pre-rename MetricsMiddleware string while the template emits
// RecordingMiddleware, so every current generated project looked metrics-less
// and extend re-appended the middleware.
func TestDetectGeneratedMiddlewaresSeesCurrentMetricsString(t *testing.T) {
	dir := t.TempDir()
	endpointDir := filepath.Join(dir, "endpoint", "userservice")
	if err := os.MkdirAll(endpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chain := `package userservice

func generatedMiddlewareChain() []endpoint.Middleware {
	var middlewares []endpoint.Middleware
	middlewares = append(middlewares, endpoint.TracingMiddleware())
	middlewares = append(middlewares, endpoint.ErrorHandlingMiddleware(name))
	middlewares = append(middlewares, endpoint.RecordingMiddleware(name, generatedMetricsCollector))
	return middlewares
}
`
	if err := os.WriteFile(filepath.Join(endpointDir, "generated_chain.go"), []byte(chain), 0o644); err != nil {
		t.Fatal(err)
	}

	services := []ExistingService{{
		Name:         "UserService",
		EndpointFile: filepath.Join(endpointDir, "endpoints.go"),
	}}
	got := detectGeneratedMiddlewares(services)

	want := map[string]bool{"tracing": true, "error-handling": true, "metrics": true}
	if len(got) != len(want) {
		t.Fatalf("detected %v, want the three current middlewares", got)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected detected middleware %q", name)
		}
	}
}

// TestDetectGeneratedMiddlewaresStillRecognizesTheOldString keeps projects
// generated before the rename detectable, so extend does not duplicate the
// older MetricsMiddleware either.
func TestDetectGeneratedMiddlewaresStillRecognizesTheOldString(t *testing.T) {
	dir := t.TempDir()
	endpointDir := filepath.Join(dir, "endpoint", "userservice")
	if err := os.MkdirAll(endpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chain := `package userservice

func generatedMiddlewareChain() []endpoint.Middleware {
	return []endpoint.Middleware{endpoint.MetricsMiddleware(generatedMetrics(name))}
}
`
	if err := os.WriteFile(filepath.Join(endpointDir, "generated_chain.go"), []byte(chain), 0o644); err != nil {
		t.Fatal(err)
	}

	got := detectGeneratedMiddlewares([]ExistingService{{
		Name:         "UserService",
		EndpointFile: filepath.Join(endpointDir, "endpoints.go"),
	}})
	found := false
	for _, name := range got {
		if name == "metrics" {
			found = true
		}
	}
	if !found {
		t.Fatalf("old MetricsMiddleware form not detected: %v", got)
	}
}
