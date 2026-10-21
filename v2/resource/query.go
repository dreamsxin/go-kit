package resource

import (
	"fmt"
	"math"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Sort orders a result by one field. Several Sorts are applied in order, the
// first being the most significant.
type Sort struct {
	// Field names the schema field to order by.
	Field string
	// Descending reverses the order.
	Descending bool
}

// Asc orders a result by a field in ascending order.
func Asc(field string) Sort { return Sort{Field: field} }

// Desc orders a result by a field in descending order.
func Desc(field string) Sort { return Sort{Field: field, Descending: true} }

// Page defaults and bounds, matching the HTTP transport's page and size
// parameters so one request means one window wherever it is read.
const (
	// DefaultPageNumber is the window used when the caller names none.
	DefaultPageNumber = 1
	// DefaultPageSize is the window size used when the caller names none.
	DefaultPageSize = 20
	// MaxPageSize bounds a requested window.
	MaxPageSize = 100
)

// Window is a requested result window. Number is 1-based.
//
// A zero field means "the caller did not choose"; Normalized turns that into the
// default. A negative number or a size above MaxPageSize is an error, not a
// clamp: a client that asked for a thousand rows has a bug, and answering with
// a hundred teaches it nothing.
type Window struct {
	// Number is the 1-based page number.
	Number int
	// Size is the number of records per page.
	Size int
}

// DefaultWindow returns the window used when a caller expresses no preference.
func DefaultWindow() Window {
	return Window{Number: DefaultPageNumber, Size: DefaultPageSize}
}

// Validate rejects a window a query cannot mean.
func (w Window) Validate() error {
	if w.Number < 0 {
		return apperror.InvalidArgument("resource.page_invalid",
			fmt.Sprintf("page number must be at least 1, got %d", w.Number))
	}
	if w.Size < 0 {
		return apperror.InvalidArgument("resource.page_size_invalid",
			fmt.Sprintf("page size must be at least 1, got %d", w.Size))
	}
	if w.Size > MaxPageSize {
		return apperror.InvalidArgument("resource.page_size_invalid",
			fmt.Sprintf("page size must not exceed %d, got %d", MaxPageSize, w.Size))
	}
	return nil
}

// Normalized replaces an unset field with its default.
func (w Window) Normalized() Window {
	if w.Number < 1 {
		w.Number = DefaultPageNumber
	}
	if w.Size < 1 {
		w.Size = DefaultPageSize
	}
	return w
}

// Offset returns how many matching records precede the window.
//
// The multiplication is guarded: a page number near the integer limit overflows,
// and an overflowing offset is a negative slice index — a panic in the one place
// a client can reach with a single parameter.
func (w Window) Offset() int64 {
	normalized := w.Normalized()
	pages := int64(normalized.Number - 1)
	if pages <= 0 {
		return 0
	}
	size := int64(normalized.Size)
	if pages > math.MaxInt64/size {
		return math.MaxInt64
	}
	return pages * size
}

// Page is one window of a result set.
//
// The wire shape is deliberately the same as the HTTP transport's PageResult:
// {"items","total","page","size","has_next"}. Two pagination contracts in one
// framework would mean two things for a client to learn and two places for them
// to drift apart.
type Page struct {
	// Items holds the records in this window, never nil: an empty page marshals
	// as [] rather than null, because a client that has to distinguish "no
	// rows" from "no answer" will eventually get it wrong.
	Items []Record `json:"items"`
	// Total is how many records match the filter, before the window is applied.
	Total int `json:"total"`
	// Number is the 1-based page number this window represents.
	Number int `json:"page"`
	// Size is the window size that was applied.
	Size int `json:"size"`
	// HasNext reports whether records follow this window.
	HasNext bool `json:"has_next"`
}

// NewPage assembles one window of a result set, deriving HasNext from the total.
//
// HasNext is computed by subtraction rather than by comparing page×size with the
// total: the product overflows for a page number a client is free to send, and
// an overflowed comparison reports the last page as if more followed.
func NewPage(window Window, total int, items []Record) Page {
	normalized := window.Normalized()
	if items == nil {
		items = []Record{}
	}
	offset := normalized.Offset()
	return Page{
		Items:   items,
		Total:   total,
		Number:  normalized.Number,
		Size:    normalized.Size,
		HasNext: int64(total) > offset && int64(total)-offset > int64(normalized.Size),
	}
}

// Query is one read request: which records, in what order, how much of each, and
// which window.
type Query struct {
	// Filter selects records. The zero Filter selects all of them.
	Filter Filter
	// Sort orders the result; empty leaves the order to the repository, which
	// must then be stable across pages.
	Sort []Sort
	// Fields projects the response. nil means every readable field; a non-nil
	// empty slice means none, which no request may ask for and which a merge
	// reports as a denial instead.
	Fields []string
	// Appends names relations to expand. No repository in this version expands
	// them, and one that cannot must say so rather than return records that
	// silently lack the expansion. Merging unions this list.
	Appends []string
	// Window is the page to return. Zero means the default window.
	Window Window
}

// Validate checks a query against the resource it is addressed to, so a request
// naming a column that does not exist fails before any storage is touched.
func (q Query) Validate(schema Schema) error {
	if err := q.Window.Validate(); err != nil {
		return err
	}
	if err := q.Filter.Validate(schema); err != nil {
		return err
	}
	for _, order := range q.Sort {
		field, ok := schema.Field(order.Field)
		if !ok {
			return apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("sort field %q is not part of this resource", order.Field))
		}
		if !field.Sortable {
			return apperror.InvalidArgument("resource.field_not_sortable",
				fmt.Sprintf("field %q cannot be sorted", order.Field))
		}
	}
	if q.Fields != nil && len(q.Fields) == 0 {
		return apperror.InvalidArgument("resource.fields_empty",
			"a projection that names no field is not a request for a record; omit fields to get the readable ones")
	}
	for _, name := range q.Fields {
		if _, ok := schema.Field(name); !ok {
			return apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("projection field %q is not part of this resource", name))
		}
	}
	return nil
}

// Params is one layer's contribution to a resource operation, and the value the
// layers are merged into.
//
// A request is assembled from three layers in order: the resource's defaults,
// what the client asked for, and what the policy allows. Merging them is the
// point at which a permission model either holds or leaks, so every rule below
// is written to narrow and never to widen.
type Params struct {
	// Filter selects records.
	Filter Filter
	// Sort orders the result; a layer that sets it replaces the earlier one.
	Sort []Sort
	// Fields projects the response; layers intersect.
	Fields []string
	// Appends names relations to expand; layers union.
	Appends []string
	// Window is the page to return; a layer can only lower the size.
	Window Window
	// Writable narrows which fields a write may name.
	Writable WriteMask
}

// Query returns the read request these params describe.
func (p Params) Query() Query {
	return Query{
		Filter:  p.Filter,
		Sort:    append([]Sort(nil), p.Sort...),
		Fields:  cloneFields(p.Fields),
		Appends: cloneFields(p.Appends),
		Window:  p.Window.Normalized(),
	}
}

// Validate checks the merged params against a schema, including the field names
// a write mask mentions: a deny list with a typo in it is a permission that
// silently does not apply.
func (p Params) Validate(schema Schema) error {
	if err := p.Query().Validate(schema); err != nil {
		return err
	}
	for _, name := range append(cloneFields(p.Writable.Allow), p.Writable.Deny...) {
		if _, ok := schema.Field(name); !ok {
			return apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("write mask names field %q, which is not part of this resource", name))
		}
	}
	return nil
}

// MergeParams applies the layers in order and returns their intersection.
//
//	key      rule        why
//	filter   and         a client adds conditions; it cannot remove the policy's
//	fields   intersect   a client may ask for fewer columns, never for more
//	appends  union       expanding a relation is additive and carries no authority
//	sort     overwrite   ordering has no security meaning; the last layer decides
//	window   min size    page size is the only dimension of a denial-of-service
//	writable intersect   whitelists intersect, blacklists union
//
// Stable: resource.params-merge — a later layer narrows a filter, a projection, a writable set and a page size, and never widens any of them; two disjoint projections are a denial rather than an unrestricted one.
// Covered by: TestMergeParamsKeepsEveryFilterLayer, TestMergeParamsIntersectsProjections, TestMergeParamsRejectsDisjointProjections, TestMergeWriteMasksIntersectsAllowAndUnionsDeny
func MergeParams(layers ...Params) (Params, error) {
	var merged Params
	for _, layer := range layers {
		if err := layer.Window.Validate(); err != nil {
			return Params{}, err
		}
		merged.Filter = And(merged.Filter, layer.Filter)

		fields := intersectFields(merged.Fields, layer.Fields)
		if merged.Fields != nil && layer.Fields != nil && len(fields) == 0 {
			// Two whitelists that share nothing mean the request may see no
			// column. Returning an empty record would answer 200 to a request
			// that is in fact refused, so it is refused here.
			return Params{}, apperror.PermissionDenied("resource.fields_disjoint",
				"the requested columns and the allowed columns do not overlap")
		}
		merged.Fields = fields

		merged.Appends = unionFields(merged.Appends, layer.Appends)
		if len(layer.Sort) > 0 {
			merged.Sort = append([]Sort(nil), layer.Sort...)
		}
		merged.Window = mergeWindow(merged.Window, layer.Window)
		merged.Writable = MergeWriteMasks(merged.Writable, layer.Writable)
	}
	return merged, nil
}

// mergeWindow takes the page number from whoever states it and the smallest size
// anyone asks for, because a page size is the one dimension a client must not be
// able to raise past what a policy allows.
func mergeWindow(base, layer Window) Window {
	if layer.Number > 0 {
		base.Number = layer.Number
	}
	if layer.Size > 0 && (base.Size == 0 || layer.Size < base.Size) {
		base.Size = layer.Size
	}
	return base
}
