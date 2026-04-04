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
		{"id", "Id"},
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
	if idField.Name != "Id" {
		t.Errorf("field[0].Name = %q, want Id", idField.Name)
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
