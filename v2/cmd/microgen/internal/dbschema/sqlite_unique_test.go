package dbschema

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestSQLiteIntrospectionKeepsUniqueConstraints is the reason a generated
// project can enforce what the database declared: SQLite reports uniqueness
// through an index, not through table_info, so an introspector that only reads
// column metadata silently loses every UNIQUE constraint. The generated model
// then has no uniqueIndex, -auto-migrate builds a table the source schema never
// had, and the repository's ErrDuplicated branch becomes unreachable.
func TestSQLiteIntrospectionKeepsUniqueConstraints(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "schema.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE
);
CREATE TABLE memberships (
	id INTEGER PRIMARY KEY,
	team TEXT NOT NULL,
	member TEXT NOT NULL,
	UNIQUE(team, member)
);
`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	introspector, err := NewIntrospector("sqlite")
	if err != nil {
		t.Fatalf("NewIntrospector: %v", err)
	}
	schemas, err := introspector.Tables(db, "", nil)
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	columns := map[string]map[string]ColumnInfo{}
	for _, schema := range schemas {
		columns[schema.TableName] = map[string]ColumnInfo{}
		for _, column := range schema.Columns {
			columns[schema.TableName][column.Name] = column
		}
	}

	if got := columns["users"]["email"]; !got.IsUnique {
		t.Errorf("users.email: IsUnique = false, want true — the DDL declares UNIQUE")
	}
	if got := columns["users"]["username"]; got.IsUnique {
		t.Errorf("users.username: IsUnique = true, want false — no constraint was declared")
	}
	if got := columns["users"]["id"]; got.IsUnique {
		t.Errorf("users.id: IsUnique = true, want false — a primary key is not a unique index")
	}
	// A composite UNIQUE(team, member) constrains the pair, not either column;
	// flagging both would generate two indexes the schema never had.
	for _, name := range []string{"team", "member"} {
		if got := columns["memberships"][name]; got.IsUnique {
			t.Errorf("memberships.%s: IsUnique = true, want false — the constraint covers both columns together", name)
		}
	}

	// The flag is only useful if it reaches the tag the generator writes.
	tag := columnToField(columns["users"]["email"]).GormTag
	if !strings.Contains(tag, "uniqueIndex") {
		t.Errorf("users.email gorm tag = %q, want it to carry uniqueIndex", tag)
	}
	if tag := columnToField(columns["memberships"]["team"]).GormTag; strings.Contains(tag, "uniqueIndex") {
		t.Errorf("memberships.team gorm tag = %q, want no uniqueIndex", tag)
	}
}
