package http_test

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

type QueryEmbedded struct {
	TraceID string `form:"trace_id,omitempty"`
}

type queryRequest struct {
	QueryEmbedded
	ID       string        `json:"id"`
	Limit    int           `form:"limit"`
	Active   bool          `json:"active"`
	Tags     []string      `json:"tag"`
	Since    time.Time     `json:"since"`
	Timeout  time.Duration `json:"timeout"`
	Optional *uint         `json:"optional,omitempty"`
	Ignored  string        `json:"-"`
}

func TestEncodePathAndQuery(t *testing.T) {
	optional := uint(9)
	since := time.Date(2026, time.July, 17, 10, 30, 0, 0, time.UTC)
	got, err := transporthttp.EncodePathAndQuery("/users/{id}?existing=yes", queryRequest{
		QueryEmbedded: QueryEmbedded{TraceID: "trace"},
		ID:            "a/b",
		Limit:         25,
		Active:        true,
		Tags:          []string{"red", "blue"},
		Since:         since,
		Timeout:       1500 * time.Millisecond,
		Optional:      &optional,
		Ignored:       "secret",
	})
	if err != nil {
		t.Fatalf("EncodePathAndQuery: %v", err)
	}
	for _, want := range []string{
		"/users/a%2Fb?",
		"active=true",
		"existing=yes",
		"limit=25",
		"optional=9",
		"since=2026-07-17T10%3A30%3A00Z",
		"tag=blue",
		"tag=red",
		"timeout=1.5s",
		"trace_id=trace",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("encoded URL %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("ignored field leaked into URL: %s", got)
	}
}

func TestEncodePathAndQueryRejectsUnsupportedField(t *testing.T) {
	_, err := transporthttp.EncodePathAndQuery("/search", struct {
		Filter map[string]string `json:"filter"`
	}{Filter: map[string]string{"a": "b"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("error = %v, want unsupported type", err)
	}
}

func TestEncodePathAndQueryRejectsMissingPathParameter(t *testing.T) {
	_, err := transporthttp.EncodePathAndQuery("/users/{id}", struct {
		ID *string `json:"id,omitempty"`
	}{})
	if err == nil || !strings.Contains(err.Error(), "unresolved path parameter") {
		t.Fatalf("error = %v, want unresolved path parameter", err)
	}
}

func TestEncodePathOnlyReplacesPlaceholders(t *testing.T) {
	got, err := transporthttp.EncodePath("/users/{id}?existing=yes", queryRequest{
		ID:     "a/b",
		Limit:  25,
		Active: true,
	})
	if err != nil {
		t.Fatalf("EncodePath: %v", err)
	}
	if got != "/users/a%2Fb?existing=yes" {
		t.Fatalf("encoded URL = %q", got)
	}
}

func TestEncodePathRejectsMissingPathParameter(t *testing.T) {
	_, err := transporthttp.EncodePath("/users/{id}", queryRequest{})
	if err == nil || !strings.Contains(err.Error(), "empty path parameter") {
		t.Fatalf("error = %v, want empty path parameter", err)
	}
}

func TestDecodeQueryRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/users/path-id?id=query-id&limit=10&active=true&tag=one&tag=two&since=2026-07-17&timeout=2s", nil)
	r.SetPathValue("id", "path-id")
	var got queryRequest
	if err := transporthttp.DecodeQueryRequest(r, &got); err != nil {
		t.Fatalf("DecodeQueryRequest: %v", err)
	}
	if got.ID != "path-id" {
		t.Fatalf("ID = %q, want path-id", got.ID)
	}
	if got.Limit != 10 || !got.Active {
		t.Fatalf("decoded scalars = %#v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "one" || got.Tags[1] != "two" {
		t.Fatalf("Tags = %#v", got.Tags)
	}
	if got.Since.Format("2006-01-02") != "2026-07-17" || got.Timeout != 2*time.Second {
		t.Fatalf("decoded time values = %#v", got)
	}
}

func TestDecodeQueryRequestReportsFieldError(t *testing.T) {
	r := httptest.NewRequest("GET", "/users?limit=invalid", nil)
	var target queryRequest
	err := transporthttp.DecodeQueryRequest(r, &target)
	var queryErr *transporthttp.QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %T %v, want QueryError", err, err)
	}
	if queryErr.Field != "Limit" || queryErr.StatusCode() != 400 {
		t.Fatalf("QueryError = %#v", queryErr)
	}
}

func TestDecodePathRequestOverridesBodyWithoutReadingQuery(t *testing.T) {
	r := httptest.NewRequest("PUT", "/users/path-id?id=query-id&limit=10", nil)
	r.SetPathValue("id", "path-id")
	target := queryRequest{ID: "body-id", Limit: 5}
	if err := transporthttp.DecodePathRequest(r, &target); err != nil {
		t.Fatalf("DecodePathRequest: %v", err)
	}
	if target.ID != "path-id" || target.Limit != 5 {
		t.Fatalf("decoded request = %#v", target)
	}
}
