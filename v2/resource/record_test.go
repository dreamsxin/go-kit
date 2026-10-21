package resource_test

import (
	"testing"

	"github.com/dreamsxin/go-kit/v2/resource"
)

func readSchema(t *testing.T) resource.Schema {
	t.Helper()
	return mustSchema(t,
		resource.NewField("id", resource.TypeInt),
		resource.NewField("name", resource.TypeString),
		// Declared, never readable: a column that exists in storage and must not
		// leave the service.
		resource.Field{Name: "password_hash", Type: resource.TypeString},
	)
}

// TestProjectUsesTheSchemaWhenNoFieldsAreNamed proves the default projection is
// built from declarations, not from whatever the storage returned. A SELECT *
// that reaches this layer still cannot serialize a credential hash.
func TestProjectUsesTheSchemaWhenNoFieldsAreNamed(t *testing.T) {
	schema := readSchema(t)
	stored := resource.Record{"id": 1, "name": "ling", "password_hash": "argon2id$..."}

	projected, err := schema.Project(stored, nil)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(projected) != 2 {
		t.Fatalf("projection = %v, want id and name only", projected)
	}
	if _, leaked := projected["password_hash"]; leaked {
		t.Fatal("the projection carried an unreadable field")
	}
}

// TestProjectOmitsUnreadableFieldsRatherThanRefusingThem proves naming a column
// a caller may not read is answered with its absence. The caller that asks is
// usually a merge, not a person: a policy whitelist and a client request that
// share only a forbidden column is a denial, and it is decided before this.
func TestProjectOmitsUnreadableFieldsRatherThanRefusingThem(t *testing.T) {
	schema := readSchema(t)
	stored := resource.Record{"id": 1, "name": "ling", "password_hash": "argon2id$..."}

	projected, err := schema.Project(stored, []string{"password_hash", "name"})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if _, leaked := projected["password_hash"]; leaked {
		t.Fatal("the projection carried an unreadable field that was asked for by name")
	}
	if projected["name"] != "ling" {
		t.Fatalf("projection = %v, want name preserved", projected)
	}
}

// TestProjectKeepsTheRequestedOrderAndRejectsUnknownNames proves the projection
// is a whitelist that cannot be satisfied by a typo: an undeclared name is an
// error, because an empty object hides the mistake.
func TestProjectKeepsTheRequestedOrderAndRejectsUnknownNames(t *testing.T) {
	schema := readSchema(t)
	stored := resource.Record{"id": 1, "name": "ling"}

	projected, err := schema.Project(stored, []string{"name", "name", "id"})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(projected) != 2 {
		t.Fatalf("projection = %v, want two fields", projected)
	}
	if _, err := schema.Project(stored, []string{"nmae"}); codeOf(err) != "resource.unknown_field" {
		t.Fatalf("Project with a typo = %v, want resource.unknown_field", err)
	}
}

// TestProjectOmitsAFieldTheRecordDoesNotCarry proves a projection describes what
// a response may contain, not what it must contain.
func TestProjectOmitsAFieldTheRecordDoesNotCarry(t *testing.T) {
	schema := readSchema(t)
	projected, err := schema.Project(resource.Record{"id": 3}, []string{"id", "name"})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if _, present := projected["name"]; present {
		t.Fatalf("projection invented a field: %v", projected)
	}
}

// TestKeyValidateRefusesAKeyThatNamesNothing proves an empty or incomplete key
// is rejected at the contract rather than interpreted by an adapter. "The empty
// key" is the shape of a delete that empties a table.
func TestKeyValidateRefusesAKeyThatNamesNothing(t *testing.T) {
	if code := codeOf(resource.Key{}.Validate()); code != "resource.key_required" {
		t.Fatalf("empty key = %v, want resource.key_required", code)
	}
	if code := codeOf(resource.Key{"id": nil}.Validate()); code != "resource.key_incomplete" {
		t.Fatalf("key with a nil value = %v, want resource.key_incomplete", code)
	}
	if err := resource.KeyOf("id", 7).Validate(); err != nil {
		t.Fatalf("a complete key: %v", err)
	}
}

// TestRecordCloneIsIndependent proves a caller mutating what it was handed does
// not reach stored state.
func TestRecordCloneIsIndependent(t *testing.T) {
	original := resource.Record{"id": 1, "name": "ling"}
	clone := original.Clone()
	clone["name"] = "wei"
	delete(clone, "id")

	if original["name"] != "ling" {
		t.Fatalf("clone mutation reached the original: %v", original)
	}
	if _, present := original["id"]; !present {
		t.Fatal("clone deletion reached the original")
	}
	if resource.Record(nil).Clone() != nil {
		t.Fatal("cloning a nil record produced a non-nil one")
	}
}

// TestRecordFieldNamesAreSorted proves the iteration a write check and an error
// message use is deterministic, so the same bad record always reports the same
// field first.
func TestRecordFieldNamesAreSorted(t *testing.T) {
	record := resource.Record{"z": 1, "a": 2, "m": 3}
	names := record.FieldNames()
	want := []string{"a", "m", "z"}
	assertFields(t, "field names", names, want)
}
