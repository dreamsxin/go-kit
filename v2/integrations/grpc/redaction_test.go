package grpc_test

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpckit "github.com/dreamsxin/go-kit/v2/integrations/grpc"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// A gateway relaying an upstream gRPC failure must not put the upstream
// description in its own response body, the same rule client.HTTPStatusError
// already follows for HTTP.
func TestStatusErrorRedactsTheUpstreamDescription(t *testing.T) {
	upstream := status.Error(codes.NotFound, "user 42 missing from shard db-7 (dsn=postgres://svc:pw@10.0.0.9)")

	classified := grpckit.ClassifyError(upstream)

	var messager transporthttp.PublicMessager
	if !errors.As(classified, &messager) {
		t.Fatal("StatusError must state a public message")
	}
	public := messager.PublicMessage()
	if strings.Contains(public, "shard db-7") || strings.Contains(public, "10.0.0.9") {
		t.Fatalf("public message leaked the description: %q", public)
	}
	if !strings.Contains(public, codes.NotFound.String()) {
		t.Fatalf("public message should name the code, got %q", public)
	}

	// Error() still keeps the detail, which is what logs need.
	if !strings.Contains(classified.Error(), "shard db-7") {
		t.Fatalf("Error() should keep the upstream detail for logs, got %q", classified.Error())
	}
}
