package resource

import (
	"context"
	"fmt"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Repository is the storage contract a resource route is handed.
//
// The interface belongs to this package and the implementation belongs to
// whoever owns the storage: an application's own adapter, or a separate module
// with its driver dependency. That direction is what keeps the core free of a
// database driver — an application using go-kit for HTTP only should not build
// one because a query vocabulary mentioned SQL.
//
// The obligations every implementation carries:
//
//   - A filter selects records; it is never dropped, relaxed, or partially
//     applied. A condition the storage cannot express is an error, because an
//     answer to a broader question is indistinguishable from an authorization
//     failure at the call site.
//   - A projection is applied to the response, and an unreadable field is
//     omitted even when the storage returned it.
//   - A window is applied after the filter and after the sort, and Page.Total
//     counts every matching record, not the records in the window.
//   - Errors are *apperror.Error values, so a transport can map them: a missing
//     record is NotFound, a key that names nothing is InvalidArgument, a write
//     outside the mask is PermissionDenied.
type Repository interface {
	// List returns the records a query selects, in its window and projection.
	List(ctx context.Context, query Query) (Page, error)
	// Get returns the record one key identifies.
	Get(ctx context.Context, key Key) (Record, error)
	// Create stores a new record and returns it as stored — with the key the
	// repository assigned, if it assigns one.
	Create(ctx context.Context, record Record, mask WriteMask) (Record, error)
	// Update changes the fields a patch names and returns the stored record. A
	// patch is partial: a field it does not mention keeps its value.
	Update(ctx context.Context, key Key, patch Record, mask WriteMask) (Record, error)
	// Destroy removes the record one key identifies.
	Destroy(ctx context.Context, key Key) error
	// Aggregate computes one figure over the records a filter selects,
	// optionally grouped.
	Aggregate(ctx context.Context, query AggregateQuery) (AggregateResult, error)
}

// AggregateFunction names a figure an aggregate query computes.
type AggregateFunction string

// The figures an aggregate query may compute.
const (
	// AggregateCount counts matching records, or records where Field is
	// present when Field is given.
	AggregateCount AggregateFunction = "count"
	// AggregateSum adds a numeric field.
	AggregateSum AggregateFunction = "sum"
	// AggregateAverage averages a numeric field.
	AggregateAverage AggregateFunction = "avg"
	// AggregateMin returns the lowest value of an ordered field.
	AggregateMin AggregateFunction = "min"
	// AggregateMax returns the highest value of an ordered field.
	AggregateMax AggregateFunction = "max"
)

// AggregateQuery asks for one figure over a filtered set, optionally per group.
type AggregateQuery struct {
	// Function is the figure to compute.
	Function AggregateFunction
	// Field is the field it is computed over. AggregateCount may leave it
	// empty to count records; every other function requires it.
	Field string
	// Filter selects the records. The zero Filter selects all of them.
	Filter Filter
	// GroupBy splits the records into groups, one per distinct combination.
	GroupBy []string
}

// Validate checks an aggregate query against a schema.
//
// Grouping requires a readable field, which is a permission rule and not a
// storage one: the distinct values of a column are the column, so grouping by a
// field a caller may not read would answer questions that reading it is refused.
func (q AggregateQuery) Validate(schema Schema) error {
	if err := q.Filter.Validate(schema); err != nil {
		return err
	}
	switch q.Function {
	case AggregateCount, AggregateSum, AggregateAverage, AggregateMin, AggregateMax:
	default:
		return apperror.InvalidArgument("resource.aggregate_unknown",
			fmt.Sprintf("%q is not an aggregate function", string(q.Function)))
	}
	if q.Field == "" && q.Function != AggregateCount {
		return apperror.InvalidArgument("resource.aggregate_field_required",
			fmt.Sprintf("%s needs a field", q.Function))
	}
	if q.Field != "" {
		field, ok := schema.Field(q.Field)
		if !ok {
			return apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("aggregate field %q is not part of this resource", q.Field))
		}
		switch q.Function {
		case AggregateSum, AggregateAverage:
			if !numericFieldType(field.Type) {
				return apperror.InvalidArgument("resource.aggregate_not_numeric",
					fmt.Sprintf("%s needs a numeric field, and %q is %s", q.Function, field.Name, field.Type))
			}
		case AggregateMin, AggregateMax:
			if !orderedFieldType(field.Type) {
				return apperror.InvalidArgument("resource.aggregate_not_ordered",
					fmt.Sprintf("%s needs an ordered field, and %q is %s", q.Function, field.Name, field.Type))
			}
		}
	}
	for _, name := range q.GroupBy {
		field, ok := schema.Field(name)
		if !ok {
			return apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("group field %q is not part of this resource", name))
		}
		if !field.Readable {
			return apperror.InvalidArgument("resource.field_not_readable",
				fmt.Sprintf("field %q cannot be grouped", name))
		}
	}
	return nil
}

// AggregateRow is one group's figure. Group is empty when the query does not
// group, and the result then holds exactly one row.
type AggregateRow struct {
	// Group holds the group's field values, one entry per GroupBy field.
	Group Record `json:"group"`
	// Value is the computed figure: an int for a count, int64 or float64 for a
	// sum, float64 for an average, and the field's own type for a min or max.
	Value any `json:"value"`
}

// AggregateResult holds one row per group, ordered by group key so that the same
// data yields the same answer twice.
type AggregateResult struct {
	// Rows holds the groups. It is never nil.
	Rows []AggregateRow `json:"rows"`
}
