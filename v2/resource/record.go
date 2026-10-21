package resource

import (
	"fmt"
	"sort"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Record is one row of a resource: field name to value.
//
// The values are whatever the storage layer returns — int64, time.Time, []byte —
// not the normalized types a filter carries. Bridging the two is the adapter's
// job, and NormalizeValue is the shared answer to it, because only the adapter
// knows what its driver hands back.
type Record map[string]any

// Clone returns a copy of the record, so a caller cannot mutate stored state by
// writing to what it was handed.
//
// The copy is one level deep: field values are immutable in every adapter this
// contract describes. An adapter storing nested documents must copy those too,
// and says so in its own documentation.
func (r Record) Clone() Record {
	if r == nil {
		return nil
	}
	clone := make(Record, len(r))
	for name, value := range r {
		clone[name] = value
	}
	return clone
}

// FieldNames returns the record's field names in sorted order, which is what
// makes an error message about a record reproducible.
func (r Record) FieldNames() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Key identifies exactly one record: a value for each field of the primary key.
//
// A composite key is a map rather than a struct because key fields are declared
// per resource, in the schema, and a struct would fix them at compile time for
// every resource at once.
type Key map[string]any

// KeyOf builds a single-field key.
func KeyOf(field string, value any) Key {
	return Key{field: value}
}

// IsZero reports whether the key names no field.
func (k Key) IsZero() bool { return len(k) == 0 }

// Validate refuses a key that does not identify one record.
//
// An empty key is not "the first record" and not "all records": a repository
// that treats it as a condition would delete or fetch everything, so it is
// rejected at the edge of the call instead of being interpreted by each adapter
// on its own.
func (k Key) Validate() error {
	if k.IsZero() {
		return apperror.InvalidArgument("resource.key_required",
			"a key must name every field of the primary key")
	}
	for field, value := range k {
		if field == "" {
			return apperror.InvalidArgument("resource.key_required",
				"a key field name cannot be empty")
		}
		if value == nil {
			return apperror.InvalidArgument("resource.key_incomplete",
				fmt.Sprintf("key field %q has no value", field))
		}
	}
	return nil
}

// WriteMask narrows which fields a write may name.
//
// Allow is the whitelist and Deny the blacklist. Their zero value is nil, which
// means "this layer has no opinion"; a non-nil empty Allow means "no field at
// all". The distinction matters: two disjoint whitelists intersect to the empty
// one, and collapsing that to "unrestricted" is how a merged permission turns
// into a write of every column. Following the rest of this package, the
// ambiguous construction — Allow: []string{} written by hand — fails closed
// (nothing is writable) rather than open.
type WriteMask struct {
	// Allow lists the writable fields, or nil when this mask sets no whitelist.
	Allow []string
	// Deny lists the fields no layer may write, whatever a whitelist says.
	Deny []string
}

// Allows reports whether one field may be written.
func (m WriteMask) Allows(field string) bool {
	if containsField(m.Deny, field) {
		return false
	}
	if m.Allow == nil {
		return true
	}
	return containsField(m.Allow, field)
}

// Check rejects a write that names a field outside the mask.
//
// The field is rejected rather than dropped. A create that silently ignores
// `role: "admin"` answers 200 and stores a record the caller did not ask for;
// the caller then believes the write happened, which is a worse outcome than a
// refusal that names the field.
func (m WriteMask) Check(record Record) error {
	for _, field := range record.FieldNames() {
		if m.Allows(field) {
			continue
		}
		return apperror.PermissionDenied("resource.field_not_writable",
			fmt.Sprintf("field %q is not writable in this request", field))
	}
	return nil
}

// MergeWriteMasks combines masks from several layers so that no layer can widen
// what another allows.
//
// Whitelists intersect and blacklists union, because those are the only two
// operations that cannot grant a field some layer refused. Applying one rule to
// both — as a per-key merge table would — either lets a deny list be shrunk by a
// later layer or lets a whitelist be extended by it.
func MergeWriteMasks(masks ...WriteMask) WriteMask {
	var merged WriteMask
	for _, mask := range masks {
		merged.Allow = intersectFields(merged.Allow, mask.Allow)
		merged.Deny = unionFields(merged.Deny, mask.Deny)
	}
	return merged
}
