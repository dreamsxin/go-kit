package resource

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// FieldType is the value domain of a field, which is what decides whether an
// operator may be used on it.
type FieldType string

// The value domains a Field may declare.
const (
	// TypeString holds text. It is the only type OpLike and OpPrefix apply to.
	TypeString FieldType = "string"
	// TypeInt holds a signed integer. Values arrive as json.Number so an
	// identifier beyond 2^53 survives the trip.
	TypeInt FieldType = "int"
	// TypeFloat holds a floating-point number.
	TypeFloat FieldType = "float"
	// TypeBool holds a boolean.
	TypeBool FieldType = "bool"
	// TypeTime holds an instant. Values arrive as RFC 3339 text.
	TypeTime FieldType = "time"
	// TypeJSON holds an opaque JSON document — a column a field type does not
	// describe. Comparisons other than equality on it are rejected: ordering an
	// arbitrary document is not a question a query can answer portably.
	TypeJSON FieldType = "json"
)

// Field describes one field a resource exposes to queries: its name, its value
// domain, and what a caller is allowed to do with it.
//
// The zero Field grants nothing, and that is deliberate. A field added to a
// schema without its capabilities considered must fail closed — the alternative
// is a column that quietly became filterable, and therefore enumerable, because
// somebody copied a struct literal.
type Field struct {
	// Name is the field's name in the JSON document and in a filter.
	Name string
	// Type is its value domain.
	Type FieldType
	// Filterable allows the field in a comparison. A credential hash or an
	// internal note is not filterable, because a filter on it turns "can I read
	// this record" into "can I learn this value one query at a time".
	Filterable bool
	// Sortable allows the field in a sort. Sorting by a sensitive column leaks
	// the same information more slowly.
	Sortable bool
	// Readable allows the field in a response, and is what the default
	// projection is built from. A column that exists but must never leave the
	// service — a credential hash, an internal note — is declared without it,
	// so no response can carry it by accident.
	Readable bool
	// Writable allows the field in a create or an update. It is false on a
	// field built by NewField: writing is the capability a caller opts into,
	// while reading a field it just declared is what the declaration meant.
	Writable bool
}

// NewField declares a field that may be read, filtered and sorted, and not
// written.
//
// Reading is what declaring a field means, so it is granted here. Writing is the
// capability a caller opts into by setting Writable, because a field whose
// writability was never considered must not become assignable.
func NewField(name string, kind FieldType) Field {
	return Field{Name: name, Type: kind, Filterable: true, Sortable: true, Readable: true}
}

// Schema is the set of fields a resource exposes, in declaration order.
//
// It exists so a filter can be validated against something: a query naming a
// field the resource does not have is a mistake to report, not a condition to
// drop — the record set would otherwise answer a broader question than the one
// asked, which reads exactly like a permission failure at the call site.
type Schema struct {
	fields []Field
	index  map[string]int
}

// NewSchema builds a schema from field declarations. Duplicate names, an empty
// name, a name that starts with "$" (the dialect's operator prefix) and an
// unknown field type are errors: each one makes a query ambiguous in a way a
// later layer cannot repair.
func NewSchema(fields ...Field) (Schema, error) {
	schema := Schema{fields: append([]Field(nil), fields...), index: make(map[string]int, len(fields))}
	for i, field := range schema.fields {
		if field.Name == "" {
			return Schema{}, apperror.InvalidArgument("resource.field_name_required",
				fmt.Sprintf("field %d has no name", i))
		}
		if strings.HasPrefix(field.Name, "$") {
			return Schema{}, apperror.InvalidArgument("resource.field_name_reserved",
				fmt.Sprintf("field %q starts with $, which the filter dialect reserves for operators", field.Name))
		}
		if previous, exists := schema.index[field.Name]; exists {
			return Schema{}, apperror.InvalidArgument("resource.field_name_duplicated",
				fmt.Sprintf("field %q is declared twice (positions %d and %d)", field.Name, previous, i))
		}
		if !knownFieldType(field.Type) {
			return Schema{}, apperror.InvalidArgument("resource.field_type_unknown",
				fmt.Sprintf("field %q has unknown type %q", field.Name, string(field.Type)))
		}
		schema.index[field.Name] = i
	}
	return schema, nil
}

// Field returns the declaration of one field.
func (s Schema) Field(name string) (Field, bool) {
	position, ok := s.index[name]
	if !ok {
		return Field{}, false
	}
	return s.fields[position], true
}

// Fields returns the declarations in the order they were given.
func (s Schema) Fields() []Field {
	return append([]Field(nil), s.fields...)
}

// Len reports how many fields the schema declares.
func (s Schema) Len() int { return len(s.fields) }

// Project narrows a record to the fields a caller may read.
//
// A nil fields argument means "whatever the schema allows", and that projection
// is built from the declarations rather than from the record: a column the
// schema marks unreadable is dropped even when the storage returned it, so no
// response can carry one because a SELECT was written as `*`.
//
// A non-nil fields argument is a whitelist. A name the schema does not declare
// is an error — it is a typo or a stale client, and answering it with an empty
// object hides both. A declared but unreadable name is dropped instead: the
// caller asked for a column it may not have, and the answer to "give me this
// column" is not "here is the whole row".
func (s Schema) Project(record Record, fields []string) (Record, error) {
	projected := make(Record, len(record))
	if fields == nil {
		for _, field := range s.fields {
			if !field.Readable {
				continue
			}
			if value, present := record[field.Name]; present {
				projected[field.Name] = value
			}
		}
		return projected, nil
	}
	added := make(map[string]struct{}, len(fields))
	for _, name := range fields {
		field, ok := s.Field(name)
		if !ok {
			return nil, apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("field %q is not part of this resource", name))
		}
		if !field.Readable {
			continue
		}
		if _, duplicate := added[name]; duplicate {
			continue
		}
		added[name] = struct{}{}
		if value, present := record[name]; present {
			projected[name] = value
		}
	}
	return projected, nil
}

// Validate checks a filter against a schema: every named field exists, is
// filterable, every operator applies to that field's type, and every operand has
// the shape the operator requires.
//
// This is the boundary where a client's query stops being text and becomes a
// condition the repositories must honour, so nothing here is forgiving. A query
// the schema cannot describe is rejected with a code that names what was wrong;
// it is never narrowed silently, because "the condition you sent was ignored" is
// indistinguishable from an authorization bug to whoever receives the answer.
func (f Filter) Validate(schema Schema) error {
	if f.IsZero() {
		return nil
	}
	return f.validateNode(schema, "")
}

func (f Filter) validateNode(schema Schema, path string) error {
	if err := f.checkShape(); err != nil {
		return err
	}
	if f.Op.isLogical() {
		for i, child := range f.Children {
			if err := child.validateNode(schema, joinChildPath(path, i)); err != nil {
				return err
			}
		}
		return nil
	}
	field, ok := schema.Field(f.Field)
	if !ok {
		return reject(path, "filter.unknown_field",
			"field %q is not part of this resource", f.Field)
	}
	if !field.Filterable {
		return reject(path, "filter.field_not_filterable", "field %q cannot be filtered", f.Field)
	}
	return checkOperatorForField(f, field, path)
}

// checkOperatorForField answers one question: does this operator mean anything
// on this field type, and is the operand the right kind of value for it.
func checkOperatorForField(f Filter, field Field, path string) error {
	switch f.Op {
	case OpLike, OpPrefix:
		if field.Type != TypeString {
			return reject(path, "filter.operator_not_allowed",
				"%s applies to string fields, and %q is %s", f.Op, field.Name, field.Type)
		}
		return requireValues(f, field, path, []any{f.Value})
	case OpGt, OpGte, OpLt, OpLte:
		if !orderedFieldType(field.Type) {
			return reject(path, "filter.operator_not_allowed",
				"%s needs an ordered field, and %q is %s", f.Op, field.Name, field.Type)
		}
		return requireValues(f, field, path, []any{f.Value})
	case OpDateRange:
		if field.Type != TypeTime {
			return reject(path, "filter.operator_not_allowed",
				"dateRange applies to time fields, and %q is %s", field.Name, field.Type)
		}
		return requireValues(f, field, path, elements(f.Value))
	case OpIn, OpNotIn:
		return requireValues(f, field, path, elements(f.Value))
	case OpEmpty, OpNotEmpty:
		return nil
	case OpEq, OpNe:
		if field.Type == TypeJSON {
			// Comparing a document for equality is only meaningful where the
			// adapter can canonicalize it; refusing is honest, and a field that
			// needs comparison is not an opaque document.
			return reject(path, "filter.operator_not_allowed",
				"%s cannot compare the opaque field %q", f.Op, field.Name)
		}
		return requireValues(f, field, path, []any{f.Value})
	default:
		return reject(path, "filter.unknown_operator", "%q is not an operator", string(f.Op))
	}
}

// requireValues rejects the first operand that does not fit the field's domain.
func requireValues(f Filter, field Field, path string, values []any) error {
	for _, value := range values {
		if !valueFits(field.Type, value) {
			return reject(path, "filter.type_mismatch",
				"%s on field %q expects %s, got %s (%v)",
				f.Op, field.Name, field.Type, describeValue(value), value)
		}
	}
	return nil
}

// elements returns the operands of a list-valued comparison, tolerating both the
// []any a parsed filter carries and the typed slice a caller built in Go.
func elements(value any) []any {
	list, ok := listLen(value)
	if !ok {
		return nil
	}
	reflected := reflect.ValueOf(value)
	result := make([]any, 0, list)
	for i := 0; i < list; i++ {
		result = append(result, reflected.Index(i).Interface())
	}
	return result
}

// isLogical reports whether the operator takes children instead of a field.
func (o Op) isLogical() bool {
	return o == OpAnd || o == OpOr || o == OpNot
}

// isComparison reports whether the operator compares one field against a value.
// It is the closed set a field's own object may name.
func (o Op) isComparison() bool {
	switch o {
	case OpEq, OpNe, OpIn, OpNotIn, OpGt, OpGte, OpLt, OpLte, OpLike, OpPrefix, OpDateRange, OpEmpty, OpNotEmpty:
		return true
	default:
		return false
	}
}
