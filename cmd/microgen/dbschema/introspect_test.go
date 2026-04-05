package dbschema

import (
	"os"
	"testing"
)

func TestSnakeToCamel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user", "User"},
		{"user_profile", "UserProfile"},
		{"order_item_detail", "OrderItemDetail"},
		{"id", "ID"},       // Go initialism
		{"user_id", "UserID"},
		{"url", "URL"},
		{"uri", "URI"},
		{"ip", "IP"},
		{"http", "HTTP"},
	}
	for _, c := range cases {
		got := snakeToCamel(c.in)
		if got != c.want {
			t.Errorf("snakeToCamel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDbTypeToGoType(t *testing.T) {
	cases := []struct {
		dbType   string
		nullable bool
		want     string
	}{
		{"int", false, "int"},
		{"int", true, "*int"},
		{"bigint", false, "int64"},
		{"varchar(64)", false, "string"},
		{"text", false, "string"},
		{"tinyint", false, "bool"},
		{"datetime", false, "time.Time"},
		{"timestamp", true, "*time.Time"},
		{"float", false, "float32"},
		{"decimal(10,2)", false, "float64"},
		{"json", false, "string"},
		{"blob", false, "[]byte"},
		{"bytea", false, "[]byte"},
		{"smallint", false, "int16"},
		{"mediumint", false, "int"},
	}
	for _, c := range cases {
		got := dbTypeToGoType(c.dbType, c.nullable)
		if got != c.want {
			t.Errorf("dbTypeToGoType(%q, %v) = %q, want %q", c.dbType, c.nullable, got, c.want)
		}
	}
}

func TestSingularize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"users", "user"},
		{"categories", "category"},
		{"people", "person"},
		{"orders", "order"},
		{"addresses", "address"},
		{"boxes", "box"},
		{"quizzes", "quiz"},
		{"dishes", "dish"},
		{"status", "status"},
		{"", ""},
	}
	for _, c := range cases {
		got := singularize(c.in)
		if got != c.want {
			t.Errorf("singularize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewIntrospector(t *testing.T) {
	drivers := []string{"mysql", "postgres", "sqlite", "sqlserver"}
	for _, d := range drivers {
		_, err := NewIntrospector(d)
		if err != nil {
			t.Errorf("NewIntrospector(%q) error: %v", d, err)
		}
	}
	_, err := NewIntrospector("oracle")
	if err == nil {
		t.Error("NewIntrospector(\"oracle\") should return error")
	}
}

func TestTableToModel(t *testing.T) {
	schema := &TableSchema{
		TableName: "user_profile",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
			{Name: "user_name", DBType: "varchar(64)", IsNullable: false, Comment: "用户名"},
			{Name: "score", DBType: "int", IsNullable: true},
			{Name: "created_at", DBType: "datetime", IsNullable: true},
		},
	}
	model := tableToModel(schema)

	if model.Name != "UserProfile" {
		t.Errorf("model.Name = %q, want UserProfile", model.Name)
	}
	if len(model.Fields) != 3 {
		t.Errorf("len(fields) = %d, want 3", len(model.Fields))
	}
}

func TestToParseResult(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "users",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "name", DBType: "varchar(64)"},
			},
		},
	}

	result := ToParseResult(schemas, "ShopService", "shop")

	if result.PackageName != "shop" {
		t.Errorf("PackageName = %q, want shop", result.PackageName)
	}
	if len(result.Services) != 1 {
		t.Errorf("len(Services) = %d, want 1", len(result.Services))
	}
}

func TestWriteIDL(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "products",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "name", DBType: "varchar(128)", IsNullable: false},
			},
		},
	}

	dir := t.TempDir()
	idlPath, err := WriteIDL(schemas, "shop", dir)
	if err != nil {
		t.Fatalf("WriteIDL error: %v", err)
	}

	if _, err := os.Stat(idlPath); os.IsNotExist(err) {
		t.Fatal("idl.go was not created")
	}
}

func TestModelToSchema(t *testing.T) {
	original := &TableSchema{
		TableName: "orders",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
			{Name: "user_id", DBType: "int", IsNullable: false},
		},
	}

	model := tableToModel(original)
	roundTripped := ModelToSchema(model)

	if roundTripped.TableName != original.TableName {
		t.Errorf("TableName = %q, want %q", roundTripped.TableName, original.TableName)
	}
}
