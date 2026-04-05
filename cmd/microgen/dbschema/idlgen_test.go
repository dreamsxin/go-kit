package dbschema

import (
	"os"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// ── WriteIDL ─────────────────────────────────────────────────────────────────

func TestWriteIDL_BasicSchema(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "users",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "username", DBType: "varchar(64)", IsNullable: false, Comment: "用户名"},
				{Name: "email", DBType: "varchar(128)", IsNullable: false},
				{Name: "score", DBType: "float", IsNullable: true},
			},
		},
	}

	dir := t.TempDir()
	idlPath, err := WriteIDL(schemas, "shop", dir)
	if err != nil {
		t.Fatalf("WriteIDL: %v", err)
	}

	content, err := os.ReadFile(idlPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)

	// package 声明
	if !strings.HasPrefix(strings.TrimSpace(s), "package shop") {
		t.Errorf("expected package shop, got: %s", s[:minLen(50, len(s))])
	}

	// GORM model
	if !strings.Contains(s, "type User struct") {
		t.Error("should contain User struct")
	}

	// DTOs
	if !strings.Contains(s, "type CreateUserRequest struct") {
		t.Error("should contain CreateUserRequest")
	}
	if !strings.Contains(s, "type GetUserRequest struct") {
		t.Error("should contain GetUserRequest")
	}
	if !strings.Contains(s, "type UpdateUserRequest struct") {
		t.Error("should contain UpdateUserRequest")
	}
	if !strings.Contains(s, "type DeleteUserRequest struct") {
		t.Error("should contain DeleteUserRequest")
	}
	if !strings.Contains(s, "type ListUsersRequest struct") {
		t.Error("should contain ListUsersRequest")
	}

	// Service interface
	if !strings.Contains(s, "type ShopService interface") {
		t.Error("should contain ShopService interface")
	}
	if !strings.Contains(s, "CreateUser(ctx context.Context") {
		t.Error("should contain CreateUser method")
	}
}

func TestWriteIDL_MultipleSchemas(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "products",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "name", DBType: "varchar(128)"},
			},
		},
		{
			TableName: "orders",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "product_id", DBType: "int"},
			},
		},
	}

	dir := t.TempDir()
	_, err := WriteIDL(schemas, "store", dir)
	if err != nil {
		t.Fatalf("WriteIDL: %v", err)
	}

	content, _ := os.ReadFile(dir + "/idl.go")
	s := string(content)

	if !strings.Contains(s, "type Product struct") {
		t.Error("should contain Product struct")
	}
	if !strings.Contains(s, "type Order struct") {
		t.Error("should contain Order struct")
	}
	if !strings.Contains(s, "CreateProduct") {
		t.Error("should contain CreateProduct method")
	}
	if !strings.Contains(s, "CreateOrder") {
		t.Error("should contain CreateOrder method")
	}
}

func TestWriteIDL_WithTimeField(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "events",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true},
				{Name: "happened_at", DBType: "datetime"},
			},
		},
	}

	dir := t.TempDir()
	_, err := WriteIDL(schemas, "events", dir)
	if err != nil {
		t.Fatalf("WriteIDL: %v", err)
	}

	content, _ := os.ReadFile(dir + "/idl.go")
	s := string(content)

	// time.Time フィールドがある場合は "time" パッケージをインポートする
	if !strings.Contains(s, `"time"`) {
		t.Error("should import time package when time.Time fields exist")
	}
}

func TestWriteIDL_EmptyPkgName_DefaultsToIDL(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "items",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true},
			},
		},
	}

	dir := t.TempDir()
	_, err := WriteIDL(schemas, "", dir)
	if err != nil {
		t.Fatalf("WriteIDL: %v", err)
	}

	content, _ := os.ReadFile(dir + "/idl.go")
	if !strings.HasPrefix(strings.TrimSpace(string(content)), "package idl") {
		t.Error("empty pkgName should default to 'idl'")
	}
}

// ── buildTag ──────────────────────────────────────────────────────────────────

func TestBuildTag_BothTags(t *testing.T) {
	got := buildTag("username", "column:username;not null")
	if !strings.Contains(got, `json:"username"`) {
		t.Errorf("buildTag: missing json tag, got %q", got)
	}
	if !strings.Contains(got, `gorm:"column:username;not null"`) {
		t.Errorf("buildTag: missing gorm tag, got %q", got)
	}
}

func TestBuildTag_JSONOnly(t *testing.T) {
	got := buildTag("name", "")
	if !strings.Contains(got, `json:"name"`) {
		t.Errorf("buildTag: missing json tag, got %q", got)
	}
	if strings.Contains(got, "gorm:") {
		t.Errorf("buildTag: should not contain gorm tag, got %q", got)
	}
}

func TestBuildTag_Empty(t *testing.T) {
	got := buildTag("", "")
	if got != "" {
		t.Errorf("buildTag with empty inputs: want %q, got %q", "", got)
	}
}

// ── toServiceName ─────────────────────────────────────────────────────────────

func TestToServiceName_Basic(t *testing.T) {
	cases := []struct{ in, want string }{
		{"shop", "ShopService"},
		{"user_service", "UserService"},   // already has Service suffix via snakeToCamel
		{"order", "OrderService"},
		{"", "Service"},
	}
	for _, c := range cases {
		got := toServiceName(c.in)
		if got != c.want {
			t.Errorf("toServiceName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── SnakeToCamel (exported) ───────────────────────────────────────────────────

func TestSnakeToCamel_Exported(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user_id", "UserID"},
		{"product_name", "ProductName"},
		{"http_client", "HTTPClient"},
		{"api_key", "APIKey"},
		{"created_at", "CreatedAt"},
	}
	for _, c := range cases {
		got := SnakeToCamel(c.in)
		if got != c.want {
			t.Errorf("SnakeToCamel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── ModelToSchema round-trip ──────────────────────────────────────────────────

func TestModelToSchema_RoundTrip(t *testing.T) {
	original := &TableSchema{
		TableName: "products",
		Columns: []ColumnInfo{
			{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
			{Name: "name", DBType: "varchar(128)", IsNullable: false},
			{Name: "price", DBType: "decimal", IsNullable: true},
		},
	}

	model := tableToModel(original)
	roundTripped := ModelToSchema(model)

	if roundTripped.TableName != original.TableName {
		t.Errorf("TableName: got %q, want %q", roundTripped.TableName, original.TableName)
	}
	if len(roundTripped.Columns) != len(model.Fields) {
		t.Errorf("Columns count: got %d, want %d", len(roundTripped.Columns), len(model.Fields))
	}

	// 主键应保留
	var hasPK bool
	for _, col := range roundTripped.Columns {
		if col.IsPrimary {
			hasPK = true
		}
	}
	if !hasPK {
		t.Error("round-tripped schema should have a primary key column")
	}
}

func TestModelToSchema_ColumnNameFromGormTag(t *testing.T) {
	model := &parser.Model{
		Name:      "User",
		TableName: "users",
		Fields: []parser.ModelField{
			{
				Name:    "UserName",
				Type:    "string",
				JSONTag: "user_name",
				GormTag: "column:user_name;not null",
			},
		},
	}
	schema := ModelToSchema(model)
	if len(schema.Columns) == 0 {
		t.Fatal("expected at least one column")
	}
	if schema.Columns[0].Name != "user_name" {
		t.Errorf("column name: got %q, want %q", schema.Columns[0].Name, "user_name")
	}
}

// ── extractGormTagValue ───────────────────────────────────────────────────────

func TestExtractGormTagValue(t *testing.T) {
	cases := []struct {
		tag, key, want string
	}{
		{"column:user_name;not null", "column", "user_name"},
		{"primaryKey;autoIncrement", "column", ""},
		{"type:varchar(64);not null", "type", "varchar(64)"},
		{"", "column", ""},
	}
	for _, c := range cases {
		got := extractGormTagValue(c.tag, c.key)
		if got != c.want {
			t.Errorf("extractGormTagValue(%q, %q) = %q, want %q", c.tag, c.key, got, c.want)
		}
	}
}

// ── writeDTOs (via WriteIDL) ──────────────────────────────────────────────────

func TestWriteDTOs_SkipsPrimaryKey(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "items",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true, IsAutoIncr: true},
				{Name: "name", DBType: "varchar(64)"},
			},
		},
	}

	dir := t.TempDir()
	_, err := WriteIDL(schemas, "store", dir)
	if err != nil {
		t.Fatalf("WriteIDL: %v", err)
	}

	content, _ := os.ReadFile(dir + "/idl.go")
	s := string(content)

	// CreateItemRequest should NOT contain ID field (it's auto-generated)
	lines := strings.Split(s, "\n")
	inCreateReq := false
	for _, line := range lines {
		if strings.Contains(line, "type CreateItemRequest struct") {
			inCreateReq = true
		}
		if inCreateReq && strings.Contains(line, "}") {
			break
		}
		if inCreateReq && strings.TrimSpace(line) == "ID uint `json:\"id\"`" {
			t.Error("CreateItemRequest should not contain ID field")
		}
	}
}

// ── crudMethods ───────────────────────────────────────────────────────────────

func TestCrudMethods_Count(t *testing.T) {
	methods := crudMethods("User")
	if len(methods) != 5 {
		t.Errorf("crudMethods: want 5, got %d", len(methods))
	}
}

func TestCrudMethods_HTTPMethods(t *testing.T) {
	methods := crudMethods("Product")
	httpMethods := map[string]string{}
	for _, m := range methods {
		httpMethods[m.Name] = m.HTTPMethod
	}

	cases := map[string]string{
		"CreateProduct":  "post",
		"GetProduct":     "get",
		"UpdateProduct":  "put",
		"DeleteProduct":  "delete",
		"ListProducts":   "get",
	}
	for name, want := range cases {
		if got := httpMethods[name]; got != want {
			t.Errorf("method %s: HTTPMethod want %q, got %q", name, want, got)
		}
	}
}

func TestCrudMethods_Routes(t *testing.T) {
	methods := crudMethods("Order")
	routes := map[string]string{}
	for _, m := range methods {
		routes[m.Name] = m.Route
	}

	if routes["CreateOrder"] != "/order" {
		t.Errorf("CreateOrder route: got %q, want %q", routes["CreateOrder"], "/order")
	}
	if routes["GetOrder"] != "/order/{id}" {
		t.Errorf("GetOrder route: got %q, want %q", routes["GetOrder"], "/order/{id}")
	}
	if routes["ListOrders"] != "/orders" {
		t.Errorf("ListOrders route: got %q, want %q", routes["ListOrders"], "/orders")
	}
}

// ── ToParseResult ─────────────────────────────────────────────────────────────

func TestToParseResult_ServiceName(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "users",
			Columns:   []ColumnInfo{{Name: "id", DBType: "int", IsPrimary: true}},
		},
	}

	result := ToParseResult(schemas, "MyService", "myapp")
	if result.PackageName != "myapp" {
		t.Errorf("PackageName: got %q, want %q", result.PackageName, "myapp")
	}
	if len(result.Services) != 1 {
		t.Fatalf("Services: want 1, got %d", len(result.Services))
	}
	if result.Services[0].ServiceName != "MyService" {
		t.Errorf("ServiceName: got %q, want %q", result.Services[0].ServiceName, "MyService")
	}
}

func TestToParseResult_EmptyPkgName(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "items",
			Columns:   []ColumnInfo{{Name: "id", DBType: "int", IsPrimary: true}},
		},
	}

	result := ToParseResult(schemas, "ItemService", "")
	// empty pkgName → defaults to lowercase of serviceName
	if result.PackageName != "itemservice" {
		t.Errorf("PackageName: got %q, want %q", result.PackageName, "itemservice")
	}
}

func TestToParseResult_ModelsGenerated(t *testing.T) {
	schemas := []*TableSchema{
		{
			TableName: "categories",
			Columns: []ColumnInfo{
				{Name: "id", DBType: "int", IsPrimary: true},
				{Name: "name", DBType: "varchar(64)"},
			},
		},
	}

	result := ToParseResult(schemas, "CatalogService", "catalog")
	if len(result.Models) != 1 {
		t.Fatalf("Models: want 1, got %d", len(result.Models))
	}
	if result.Models[0].Name != "Category" {
		t.Errorf("Model name: got %q, want %q", result.Models[0].Name, "Category")
	}
}

// ── goTypeToDBType ────────────────────────────────────────────────────────────

func TestGoTypeToDBType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"int", "int"},
		{"int64", "bigint"},
		{"int16", "smallint"},
		{"bool", "tinyint"},
		{"float32", "float"},
		{"float64", "decimal"},
		{"time.Time", "datetime"},
		{"[]byte", "blob"},
		{"string", "varchar(255)"},
		{"*string", "varchar(255)"},
	}
	for _, c := range cases {
		got := goTypeToDBType(c.in)
		if got != c.want {
			t.Errorf("goTypeToDBType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
