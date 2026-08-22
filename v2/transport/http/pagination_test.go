package http_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

func pageRequest(t *testing.T, rawQuery string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/items?"+rawQuery, nil)
	return req
}

func TestParsePage_Defaults(t *testing.T) {
	page, err := transporthttp.ParsePage(pageRequest(t, ""))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if page.Number != transporthttp.DefaultPageNumber {
		t.Errorf("number: got %d, want default %d", page.Number, transporthttp.DefaultPageNumber)
	}
	if page.Size != transporthttp.DefaultPageSize {
		t.Errorf("size: got %d, want default %d", page.Size, transporthttp.DefaultPageSize)
	}
}

func TestParsePage_ValidParameters(t *testing.T) {
	page, err := transporthttp.ParsePage(pageRequest(t, "page=3&size=50"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if page.Number != 3 || page.Size != 50 {
		t.Errorf("page: got %+v", page)
	}
	if page.Limit() != 50 {
		t.Errorf("limit: got %d", page.Limit())
	}
	if page.Offset() != 100 {
		t.Errorf("offset: got %d, want 100", page.Offset())
	}
}

func TestParsePage_RejectsInvalidParameters(t *testing.T) {
	cases := []struct {
		query string
		field string
	}{
		{"page=abc", "page"},
		{"page=0", "page"},
		{"page=-1", "page"},
		{"size=xyz", "size"},
		{"size=0", "size"},
		{"size=101", "size"},
	}
	for _, tc := range cases {
		_, err := transporthttp.ParsePage(pageRequest(t, tc.query))
		var verr *endpoint.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("%s: want ValidationError, got %v", tc.query, err)
			continue
		}
		if len(verr.Fields) != 1 || verr.Fields[0].Field != tc.field {
			t.Errorf("%s: fields = %+v, want one %s failure", tc.query, verr.Fields, tc.field)
		}
	}
}

func TestParsePage_AcceptsMaxSize(t *testing.T) {
	page, err := transporthttp.ParsePage(pageRequest(t, "size=100"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if page.Size != transporthttp.MaxPageSize {
		t.Errorf("size: got %d, want %d", page.Size, transporthttp.MaxPageSize)
	}
}

func TestNewPageResult(t *testing.T) {
	page := transporthttp.Page{Number: 2, Size: 10}
	items := []string{"a", "b"}

	result := transporthttp.NewPageResult(page, 25, items)

	if result.Total != 25 || result.Page != 2 || result.Size != 10 {
		t.Errorf("result fields: %+v", result)
	}
	if !result.HasNext {
		t.Error("page 2 of 25 items with size 10 should have a next page")
	}

	last := transporthttp.NewPageResult(page, 20, items)
	if last.HasNext {
		t.Error("page 2 of 20 items with size 10 is the last page")
	}
}

type validListRequest struct {
	Status   string `form:"status"`
	Page     int    `form:"page"`
	Size     int    `form:"size"`
	Since    time.Time
	Interval time.Duration
	Tags     []string `form:"tags"`
}

func TestValidateQueryStruct_Valid(t *testing.T) {
	if err := transporthttp.ValidateQueryStruct[validListRequest](); err != nil {
		t.Fatalf("valid request should pass: %v", err)
	}
}

type invalidListRequest struct {
	Status string `form:"status"`
	Data   map[string]string
}

func TestValidateQueryStruct_RejectsUnsupportedTypes(t *testing.T) {
	err := transporthttp.ValidateQueryStruct[invalidListRequest]()
	if err == nil {
		t.Fatal("expected validation error for map field")
	}
	if !strings.Contains(err.Error(), "Data") {
		t.Errorf("error should name the field: %v", err)
	}
}

type skippedFieldRequest struct {
	Status  string `form:"status"`
	Ignored func() `form:"-"`
}

func TestValidateQueryStruct_SkipsDashTaggedFields(t *testing.T) {
	if err := transporthttp.ValidateQueryStruct[skippedFieldRequest](); err != nil {
		t.Fatalf("skipped field should not fail validation: %v", err)
	}
}
