package http_test

import (
	"encoding/json"
	"testing"

	"github.com/dreamsxin/go-kit/v2/resource"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// TestResourcePageMatchesTheHTTPPageResult proves the page a repository returns
// and the page a handler serializes are one contract.
//
// The two packages cannot import each other — the query vocabulary is below the
// transport, and the transport knows nothing about resources — so this test is
// the only place the shapes can be compared. Without it, "items" in one and
// "data" in the other would both look correct in isolation, and every client
// would have to learn both.
func TestResourcePageMatchesTheHTTPPageResult(t *testing.T) {
	t.Parallel()
	fromResource, err := json.Marshal(resource.NewPage(
		resource.Window{Number: 2, Size: 10},
		25,
		[]resource.Record{{"id": 1}},
	))
	if err != nil {
		t.Fatalf("Marshal(resource.Page): %v", err)
	}
	fromTransport, err := json.Marshal(transporthttp.NewPageResult(
		transporthttp.Page{Number: 2, Size: 10},
		25,
		[]map[string]any{{"id": 1}},
	))
	if err != nil {
		t.Fatalf("Marshal(http.PageResult): %v", err)
	}
	if string(fromResource) != string(fromTransport) {
		t.Fatalf("the two page shapes differ\nresource:  %s\ntransport: %s", fromResource, fromTransport)
	}
}

// TestResourceWindowBoundsMatchHTTPPagination proves the defaults and the cap are
// the same number in both packages. A request parsed by the transport and a
// window built in Go have to mean the same thing, and two constants that drift
// apart would make page 5 of 100 rows mean two different slices.
func TestResourceWindowBoundsMatchHTTPPagination(t *testing.T) {
	t.Parallel()
	if resource.DefaultPageNumber != transporthttp.DefaultPageNumber {
		t.Fatalf("default page number: resource %d, transport %d",
			resource.DefaultPageNumber, transporthttp.DefaultPageNumber)
	}
	if resource.DefaultPageSize != transporthttp.DefaultPageSize {
		t.Fatalf("default page size: resource %d, transport %d",
			resource.DefaultPageSize, transporthttp.DefaultPageSize)
	}
	if resource.MaxPageSize != transporthttp.MaxPageSize {
		t.Fatalf("page size cap: resource %d, transport %d",
			resource.MaxPageSize, transporthttp.MaxPageSize)
	}
	if got := (resource.Window{Number: 3, Size: 10}).Offset(); got != int64((transporthttp.Page{Number: 3, Size: 10}).Offset()) {
		t.Fatalf("offset for page 3 of 10: resource %d, transport %d", got, (transporthttp.Page{Number: 3, Size: 10}).Offset())
	}
}
