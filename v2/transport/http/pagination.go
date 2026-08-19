package http

import (
	"net/http"
	"strconv"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Page defaults and bounds for ParsePage.
const (
	// DefaultPageNumber is the 1-based page used when the request omits it.
	DefaultPageNumber = 1
	// DefaultPageSize is the page size used when the request omits it.
	DefaultPageSize = 20
	// MaxPageSize bounds the requested page size.
	MaxPageSize = 100
)

// Page describes a requested result window. Number is 1-based.
type Page struct {
	Number int
	Size   int
}

// ParsePage parses the page and size query parameters. Missing parameters
// select the defaults; malformed values, numbers below 1, or sizes above
// MaxPageSize return an *endpoint.ValidationError naming the offending field,
// which the HTTP transports encode as 400.
//
//	page, err := transporthttp.ParsePage(r)
//	if err != nil {
//	    return err
//	}
//	rows := query(ctx, page.Limit(), page.Offset())
func ParsePage(r *http.Request) (Page, error) {
	if r == nil || r.URL == nil {
		return Page{}, endpoint.NewValidationError("page", "missing request URL")
	}
	query := r.URL.Query()

	number := DefaultPageNumber
	if raw := query.Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Page{}, endpoint.NewValidationError("page", "must be an integer >= 1")
		}
		number = n
	}

	size := DefaultPageSize
	if raw := query.Get("size"); raw != "" {
		s, err := strconv.Atoi(raw)
		if err != nil || s < 1 {
			return Page{}, endpoint.NewValidationError("size", "must be an integer >= 1")
		}
		if s > MaxPageSize {
			return Page{}, endpoint.NewValidationError("size", "must not exceed "+strconv.Itoa(MaxPageSize))
		}
		size = s
	}

	return Page{Number: number, Size: size}, nil
}

// Limit returns the SQL LIMIT value for the page.
func (p Page) Limit() int { return p.Size }

// Offset returns the SQL OFFSET value for the page.
func (p Page) Offset() int { return (p.Number - 1) * p.Size }

// PageResult is the standard wire shape for one page of items. Use it as the
// response type of list endpoints so clients and generated SDKs see one
// pagination contract.
type PageResult[T any] struct {
	Items   []T  `json:"items"`
	Total   int  `json:"total"`
	Page    int  `json:"page"`
	Size    int  `json:"size"`
	HasNext bool `json:"has_next"`
}

// NewPageResult assembles one page of results. total is the total number of
// matching items across all pages; HasNext is derived from it.
func NewPageResult[T any](page Page, total int, items []T) PageResult[T] {
	return PageResult[T]{
		Items:   items,
		Total:   total,
		Page:    page.Number,
		Size:    page.Size,
		HasNext: page.Number*page.Size < total,
	}
}
