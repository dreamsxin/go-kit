package dbschema

import (
	"os"
	"strings"
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
	}
	for _, c := range cases {
		got := dbTypeToGoType(c.dbType, c.nullable)
		if got != c.want {
			t.Errorf("dbTypeToGoType(%q, %v) = %q, want %q", c.dbType, c.nullable, got, c.want)
		}
	}
}

func TestTableToModel(t *testing.T) {
	schema := &TableSchema{
		TableName: "user_profile",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
			{Name: "user_name", DBType: "varchar(64)", IsNullable: false, Comment: "用户名"},
			{Name: "score", DBType: "int", IsNullable: true},
			// created_at/updated_at/deleted_at 会被过滤（GORM 模板自动添加）
			{Name: "created_at", DBType: "datetime", IsNullable: true},
		},
	}
	model := tableToModel(schema)

	if model.Name != "UserProfile" {
		t.Errorf("model.Name = %q, want UserProfile", model.Name)
	}
	if model.TableName != "user_profile" {
		t.Errorf("model.TableName = %q, want user_profile", model.TableName)
	}
	// created_at 被过滤，只剩 3 个字段
	if len(model.Fields) != 3 {
		t.Errorf("len(fields) = %d, want 3", len(model.Fields))
	}

	idField := model.Fields[0]
	if idField.Name != "ID" {
		t.Errorf("field[0].Name = %q, want ID", idField.Name)
	}
	if !idField.IsPrimary {
		t.Error("field[0].IsPrimary should be true")
	}
	if !strings.Contains(idField.GormTag, "primaryKey") {
		t.Errorf("field[0].GormTag should contain primaryKey, got %q", idField.GormTag)
	}
	// 主键不应为 nullable
	if strings.HasPrefix(idField.Type, "*") {
		t.Errorf("primary key field type should not be nullable, got %q", idField.Type)
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
		{
			TableName: "orders",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "bigint", IsPrimary: true, IsAutoIncr: true},
				{Name: "user_id", DBType: "int"},
				{Name: "amount", DBType: "decimal(10,2)"},
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
	if len(result.Models) != 2 {
		t.Errorf("len(Models) = %d, want 2", len(result.Models))
	}

	svc := result.Services[0]
	if svc.ServiceName != "ShopService" {
		t.Errorf("ServiceName = %q, want ShopService", svc.ServiceName)
	}
	// 每张表 5 个 CRUD 方法
	if len(svc.Methods) != 10 {
		t.Errorf("len(Methods) = %d, want 10 (5 per table)", len(svc.Methods))
	}
}

func TestWriteIDL(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "products",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "name", DBType: "varchar(128)", IsNullable: false},
				{Name: "price", DBType: "decimal(10,2)", IsNullable: false},
				{Name: "created_at", DBType: "datetime", IsNullable: true},
			},
		},
	}

	dir := t.TempDir()
	idlPath, err := WriteIDL(schemas, "shop", dir)
	if err != nil {
		t.Fatalf("WriteIDL error: %v", err)
	}

	content, err := os.ReadFile(idlPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	src := string(content)
	checks := []string{
		"package shop",
		"type Product struct",
		"type ProductItem struct",
		"type CreateProductRequest struct",
		"type CreateProductResponse struct",
		"type GetProductRequest struct",
		"type UpdateProductRequest struct",
		"type DeleteProductRequest struct",
		"type ListProductsRequest struct",
		"type ShopService interface",
		"CreateProduct(ctx context.Context",
		"GetProduct(ctx context.Context",
		"UpdateProduct(ctx context.Context",
		"DeleteProduct(ctx context.Context",
		"ListProducts(ctx context.Context",
	}
	for _, check := range checks {
		if !strings.Contains(src, check) {
			t.Errorf("idl.go missing %q", check)
		}
	}
}

func TestModelToSchema(t *testing.T) {
	// Build a schema, convert to model, then round-trip back to schema
	original := &TableSchema{
		TableName: "orders",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
			{Name: "user_id", DBType: "int", IsNullable: false},
			{Name: "amount", DBType: "decimal(10,2)", IsNullable: false},
			{Name: "note", DBType: "varchar(255)", IsNullable: true},
		},
	}

	model := tableToModel(original)
	roundTripped := ModelToSchema(model)

	if roundTripped.TableName != original.TableName {
		t.Errorf("TableName = %q, want %q", roundTripped.TableName, original.TableName)
	}
	// original has 4 cols; tableToModel filters none (no timestamp cols here)
	if len(roundTripped.Columns) != len(model.Fields) {
		t.Errorf("column count = %d, want %d", len(roundTripped.Columns), len(model.Fields))
	}

	// Primary key must survive the round-trip
	idCol := roundTripped.Columns[0]
	if !idCol.IsPrimary {
		t.Error("id column should be primary after round-trip")
	}
	if !idCol.IsAutoIncr {
		t.Error("id column should be auto-increment after round-trip")
	}
}

func TestAddTablesWriteIDL(t *testing.T) {
	// Simulate the add-tables workflow at the WriteIDL level:
	// 1. Write initial idl.go with "users"
	// 2. Reconstruct schemas via ModelToSchema, append "orders", rewrite idl.go
	// 3. Verify both tables appear in the output

	dir := t.TempDir()

	usersSchema := &TableSchema{
		TableName: "users",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
			{Name: "name", DBType: "varchar(64)", IsNullable: false},
		},
	}
	ordersSchema := &TableSchema{
		TableName: "orders",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "bigint", IsPrimary: true, IsAutoIncr: true},
			{Name: "user_id", DBType: "int", IsNullable: false},
			{Name: "amount", DBType: "decimal(10,2)", IsNullable: false},
		},
	}

	// Step 1: initial generation
	if _, err := WriteIDL([]*TableSchema{usersSchema}, "shop", dir); err != nil {
		t.Fatalf("initial WriteIDL: %v", err)
	}

	// Step 2: simulate mergeWithExisting — reconstruct existing model and append new table
	usersModel := tableToModel(usersSchema)
	merged := []*TableSchema{
		ModelToSchema(usersModel), // existing table reconstructed from model
		ordersSchema,              // new table
	}

	if _, err := WriteIDL(merged, "shop", dir); err != nil {
		t.Fatalf("merged WriteIDL: %v", err)
	}

	// Step 3: verify
	content, err := os.ReadFile(dir + "/idl.go")
	if err != nil {
		t.Fatalf("read merged idl.go: %v", err)
	}
	src := string(content)

	for _, want := range []string{
		"type User struct",
		"type Order struct",
		"CreateUser(ctx context.Context",
		"CreateOrder(ctx context.Context",
		"ListUsers(ctx context.Context",
		"ListOrders(ctx context.Context",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("merged idl.go missing %q", want)
		}
	}
}
