package resource_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/resource"
)

// TestNewSchemaRefusesAmbiguity proves a schema is a checked declaration, not a
// list: a duplicate name, an empty one, one the dialect reserves, or a type no
// adapter understands would each make a later query mean two things at once.
func TestNewSchemaRefusesAmbiguity(t *testing.T) {
	cases := map[string]struct {
		fields []resource.Field
		code   string
	}{
		"duplicate name": {
			fields: []resource.Field{resource.NewField("status", resource.TypeString), resource.NewField("status", resource.TypeString)},
			code:   "resource.field_name_duplicated",
		},
		"no name": {
			fields: []resource.Field{resource.NewField("", resource.TypeString)},
			code:   "resource.field_name_required",
		},
		"a reserved prefix": {
			fields: []resource.Field{resource.NewField("$eq", resource.TypeString)},
			code:   "resource.field_name_reserved",
		},
		"an unknown type": {
			fields: []resource.Field{resource.NewField("blob", resource.FieldType("blob"))},
			code:   "resource.field_type_unknown",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := resource.NewSchema(testCase.fields...); codeOf(err) != testCase.code {
				t.Fatalf("NewSchema = %v, want code %s", err, testCase.code)
			}
		})
	}
}

// TestNewFieldWritesNothing proves the declaration's default is asymmetric on
// purpose: a field is readable by query the moment it is declared, and becomes
// writable only when somebody says so. A column that quietly accepted writes
// because its declaration was copied is a mass-assignment bug.
func TestNewFieldWritesNothing(t *testing.T) {
	field := resource.NewField("status", resource.TypeString)
	if !field.Filterable || !field.Sortable {
		t.Fatalf("NewField left the field unqueryable: %+v", field)
	}
	if field.Writable {
		t.Fatal("NewField made the field writable without being asked")
	}
}

// TestValidateRejectsFieldsTheResourceDoesNotExpose proves the schema is the
// authority. A query naming a field it does not have is an error, not a condition
// to ignore: ignoring it answers a broader question, which looks exactly like a
// permission failure to the caller.
func TestValidateRejectsFieldsTheResourceDoesNotExpose(t *testing.T) {
	schema := mustSchema(t, resource.NewField("status", resource.TypeString))

	unknown := resource.Condition("secret", resource.OpEq, "x")
	if err := unknown.Validate(schema); codeOf(err) != "filter.unknown_field" {
		t.Fatalf("unknown field validated: %v", err)
	}

	hidden := resource.NewField("passwordHash", resource.TypeString)
	hidden.Filterable = false
	schema = mustSchema(t, hidden)
	notFilterable := resource.Condition("passwordHash", resource.OpEq, "x")
	if err := notFilterable.Validate(schema); codeOf(err) != "filter.field_not_filterable" {
		t.Fatalf("a field declared unfilterable validated: %v", err)
	}
}

// TestValidateOperatorMatrix is the operator table as an executable statement:
// which operators mean anything on which field type. An operator accepted on a
// type it cannot order or match would reach an adapter that has to guess.
func TestValidateOperatorMatrix(t *testing.T) {
	schema := mustSchema(t,
		resource.NewField("name", resource.TypeString),
		resource.NewField("age", resource.TypeInt),
		resource.NewField("score", resource.TypeFloat),
		resource.NewField("active", resource.TypeBool),
		resource.NewField("createdAt", resource.TypeTime),
		resource.NewField("payload", resource.TypeJSON),
	)
	number := json.Number("18")
	instant := "2026-01-01T00:00:00Z"

	cases := []struct {
		name   string
		filter resource.Filter
		code   string // "" means it must validate
	}{
		{"eq on a string", resource.Condition("name", resource.OpEq, "x"), ""},
		{"eq on an int", resource.Condition("age", resource.OpEq, number), ""},
		{"eq on a bool", resource.Condition("active", resource.OpEq, true), ""},
		{"eq on a time", resource.Condition("createdAt", resource.OpEq, instant), ""},
		{"ne on an int", resource.Condition("age", resource.OpNe, number), ""},
		{"in on a string", resource.Condition("name", resource.OpIn, []any{"a", "b"}), ""},
		{"nin on an int", resource.Condition("age", resource.OpNotIn, []int{1, 2}), ""},
		{"gt on an int", resource.Condition("age", resource.OpGt, number), ""},
		{"gte on a string", resource.Condition("name", resource.OpGte, "m"), ""},
		{"lt on a time", resource.Condition("createdAt", resource.OpLt, instant), ""},
		{"lte on a float", resource.Condition("score", resource.OpLte, 4.5), ""},
		{"like on a string", resource.Condition("name", resource.OpLike, "alp"), ""},
		{"prefix on a string", resource.Condition("name", resource.OpPrefix, "al"), ""},
		{"dateRange on a time", resource.Condition("createdAt", resource.OpDateRange, []any{instant, "2026-02-01T00:00:00Z"}), ""},
		{"empty on a string", resource.Condition("name", resource.OpEmpty, nil), ""},
		{"notEmpty on an int", resource.Condition("age", resource.OpNotEmpty, nil), ""},

		{"like on an int", resource.Condition("age", resource.OpLike, "1"), "filter.operator_not_allowed"},
		{"prefix on a time", resource.Condition("createdAt", resource.OpPrefix, "20"), "filter.operator_not_allowed"},
		{"gt on a bool", resource.Condition("active", resource.OpGt, true), "filter.operator_not_allowed"},
		{"gt on an opaque document", resource.Condition("payload", resource.OpGt, "x"), "filter.operator_not_allowed"},
		{"dateRange on a string", resource.Condition("name", resource.OpDateRange, []any{instant, instant}), "filter.operator_not_allowed"},
		{"eq on an opaque document", resource.Condition("payload", resource.OpEq, map[string]any{}), "filter.operator_not_allowed"},

		{"a string on an int", resource.Condition("age", resource.OpEq, "18"), "filter.type_mismatch"},
		{"a fraction on an int", resource.Condition("age", resource.OpEq, json.Number("18.5")), "filter.type_mismatch"},
		{"a number on a string", resource.Condition("name", resource.OpEq, number), "filter.type_mismatch"},
		{"prose on a time", resource.Condition("createdAt", resource.OpEq, "yesterday"), "filter.type_mismatch"},
		{"a string on a float", resource.Condition("score", resource.OpEq, "4.5"), "filter.type_mismatch"},
		{"one bad element of an in list", resource.Condition("age", resource.OpIn, []any{json.Number("1"), "two"}), "filter.type_mismatch"},
		{"a non-time in a range", resource.Condition("createdAt", resource.OpDateRange, []any{instant, "later"}), "filter.type_mismatch"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.filter.Validate(schema)
			if testCase.code == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want it to pass", err)
				}
				return
			}
			if code := codeOf(err); code != testCase.code {
				t.Fatalf("Validate = %q (%v), want %q", code, err, testCase.code)
			}
		})
	}
}

// TestValidateAcceptsEveryOrderedSpelling proves a value may arrive in any Go
// shape that means the field's type — a caller building a constraint in Go writes
// int, a parsed request carries json.Number, and a time may be either an instant
// or the RFC 3339 text it arrived as. Rejecting one of them would push
// conversions into every caller.
func TestValidateAcceptsEveryOrderedSpelling(t *testing.T) {
	schema := mustSchema(t,
		resource.NewField("age", resource.TypeInt),
		resource.NewField("createdAt", resource.TypeTime),
	)
	values := []any{1, int64(2), uint8(3), float64(4), json.Number("5")}
	for _, value := range values {
		if err := resource.Condition("age", resource.OpEq, value).Validate(schema); err != nil {
			t.Errorf("int field rejected %T: %v", value, err)
		}
	}
	instants := []any{"2026-01-01T00:00:00Z", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	for _, value := range instants {
		if err := resource.Condition("createdAt", resource.OpEq, value).Validate(schema); err != nil {
			t.Errorf("time field rejected %T: %v", value, err)
		}
	}
}

// TestValidateZeroFilterNeedsNoSchema proves "no condition" is valid against
// anything, including a schema that declares nothing. Reading it as an error
// would make an unfiltered list impossible.
func TestValidateZeroFilterNeedsNoSchema(t *testing.T) {
	if err := (resource.Filter{}).Validate(resource.Schema{}); err != nil {
		t.Fatalf("zero filter failed validation: %v", err)
	}
}

// TestValidateReportsThePathOfAFailure proves a nested failure says where it is.
// children[1].children[0] is the difference between a fixable message and one
// that sends the reader back to the request body.
func TestValidateReportsThePathOfAFailure(t *testing.T) {
	schema := mustSchema(t, resource.NewField("age", resource.TypeInt))
	filter := resource.And(
		resource.Condition("age", resource.OpGt, json.Number("1")),
		resource.Or(
			resource.Condition("age", resource.OpLt, json.Number("9")),
			resource.Condition("age", resource.OpLike, "1"),
		),
	)
	err := filter.Validate(schema)
	if err == nil {
		t.Fatal("a like on an int field validated")
	}
	if message := err.Error(); !strings.Contains(message, "children[1].children[1]") {
		t.Fatalf("error %q does not name the failing operand", message)
	}
}
