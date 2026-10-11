package dbschema

import (
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/validation"
)

// TestConstraintsFromDBType: the DDL's own truth becomes the contract's
// constraints — varchar and char lengths bound strings, MySQL enum columns
// list their values, and types that state nothing derive nothing.
func TestConstraintsFromDBType(t *testing.T) {
	cases := []struct {
		dbType string
		check  func(validation.Constraints) bool
		want   string
	}{
		{"varchar(64)", func(c validation.Constraints) bool {
			return c.MaxLen != nil && *c.MaxLen == 64
		}, "maxLen=64"},
		{"CHAR(12)", func(c validation.Constraints) bool {
			return c.MaxLen != nil && *c.MaxLen == 12
		}, "maxLen=12"},
		{"enum('active','suspended')", func(c validation.Constraints) bool {
			return strings.Join(c.Enum, "|") == "active|suspended"
		}, "enum=active|suspended"},
		{"TEXT", func(c validation.Constraints) bool { return c.Empty() }, ""},
		{"INTEGER", func(c validation.Constraints) bool { return c.Empty() }, ""},
		{"DATETIME", func(c validation.Constraints) bool { return c.Empty() }, ""},
		{"DECIMAL(10,2)", func(c validation.Constraints) bool { return c.Empty() }, ""},
	}
	for _, tc := range cases {
		cons := ConstraintsFromDBType(tc.dbType)
		if !tc.check(cons) {
			t.Errorf("%s: constraints = %+v, want %s", tc.dbType, cons, tc.want)
			continue
		}
		// The derived constraints must round-trip into a parseable validate
		// tag — the same one the written contract copy carries.
		if tag := cons.Tag(); tag != tc.want {
			t.Errorf("%s: tag = %q, want %q", tc.dbType, tag, tc.want)
		}
	}
}
