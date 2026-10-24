// Package main demonstrates what the transport spans add up to: one request
// through the full stack — a SERVER span around the route, the endpoint
// span beneath it, and a CLIENT span on the outbound call — printed as a
// tree when the process shuts its tracer provider down.
//
// The exporter is deliberately hand-written: providers, exporters and their
// shutdown belong to the application, and a twenty-line tree printer is the
// smallest one that shows the shape. Point Setup at a real collector and the
// same tree appears there.
//
// Run:
//
//	go run ./examples/tracingtree
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	oteladapter "github.com/dreamsxin/go-kit/v2/observability/otel"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tracingtree:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out io.Writer) error {
	// The application owns the provider. This one is in memory so the demo
	// can print the tree; a deployment points an OTLP exporter at its
	// collector instead and keeps the shutdown.
	exporter := &treeExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := provider.Tracer("tracingtree")

	// The upstream this service calls: a stub whose answer becomes the
	// response body.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream says hello"))
	}))
	defer upstream.Close()

	// The outbound client: every call through it starts a CLIENT span and
	// injects the active context, so the upstream (were it traced) would
	// continue the same tree.
	client := &http.Client{Transport: oteladapter.NewRoundTripper(tracer, http.DefaultTransport)}

	// The endpoint layer: the adapter's tracing middleware starts the
	// endpoint span that everything the business code does joins.
	endpoint := oteladapter.TracingMiddleware(tracer, "GET /aggregate")(
		func(ctx context.Context, _ any) (any, error) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
			if err != nil {
				return nil, err
			}
			response, err := client.Do(request)
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				return nil, err
			}
			return string(body), nil
		},
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /aggregate", func(w http.ResponseWriter, r *http.Request) {
		answer, err := endpoint(r.Context(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(answer.(string)))
	})

	// The transport boundary: one SERVER span per request, named after the
	// route the mux matched.
	service := oteladapter.HTTPServerMiddleware(tracer)(mux)
	server := httptest.NewServer(service)
	defer server.Close()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/aggregate", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "response: %s\n", body)

	if err := provider.Shutdown(ctx); err != nil {
		return err
	}
	fmt.Fprint(out, exporter.Tree())
	return nil
}

// treeExporter is the application-owned exporter: it keeps every ended span
// and renders the parent/child shape on demand.
type treeExporter struct {
	spans []sdktrace.ReadOnlySpan
}

// ExportSpans implements sdktrace.SpanExporter. It must not block the request
// path: this one appends under the SDK's own batching.
func (e *treeExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.spans = append(e.spans, spans...)
	return nil
}

// Shutdown implements sdktrace.SpanExporter.
func (e *treeExporter) Shutdown(ctx context.Context) error { return nil }

// Tree renders the collected spans as an indented tree, children under
// parents, siblings in the order they started.
func (e *treeExporter) Tree() string {
	var b strings.Builder
	if len(e.spans) == 0 {
		return ""
	}
	b.WriteString("trace " + e.spans[0].SpanContext().TraceID().String() + "\n")

	byID := make(map[trace.SpanID]int, len(e.spans))
	for i, span := range e.spans {
		byID[span.SpanContext().SpanID()] = i
	}
	var roots []int
	children := make(map[int][]int)
	for i, span := range e.spans {
		parent := span.Parent().SpanID()
		if at, ok := byID[parent]; ok && parent.IsValid() {
			children[at] = append(children[at], i)
			continue
		}
		roots = append(roots, i)
	}
	var render func(at, depth int)
	render = func(at, depth int) {
		span := e.spans[at]
		status := ""
		if span.Status().Code == codes.Error {
			status = "  [error]"
		}
		b.WriteString(strings.Repeat("  ", depth) + "└─ " + span.SpanKind().String() + " " + span.Name() + status + "\n")
		kids := append([]int(nil), children[at]...)
		sort.Slice(kids, func(x, y int) bool { return e.spans[kids[x]].StartTime().Before(e.spans[kids[y]].StartTime()) })
		for _, kid := range kids {
			render(kid, depth+1)
		}
	}
	for _, root := range roots {
		render(root, 1)
	}
	return b.String()
}

// Ensure the module's otel import is exercised even if the wiring above
// changes; the adapter installs global defaults only through Setup, which
// this example deliberately does not call.
var _ = otel.Tracer
