package parser_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// testdataPath 返回 testdata 目录下指定文件的绝对路径。
func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

// ─────────────────────────── ParseFull ───────────────────────────

func TestParseFull_BasicIDL(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: unexpected error: %v", err)
	}

	// 包名
	if result.PackageName != "basic" {
		t.Errorf("PackageName: want %q, got %q", "basic", result.PackageName)
	}

	// 服务数量
	if len(result.Services) != 1 {
		t.Fatalf("Services count: want 1, got %d", len(result.Services))
	}

	// 模型数量（basic.go 中有 User + CreateUserRequest 等多个 struct）
	if len(result.Models) == 0 {
		t.Error("Models should not be empty")
	}
}

func TestParseFull_MultipleServicesAndModels(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("multi.go"))
	if err != nil {
		t.Fatalf("ParseFull: unexpected error: %v", err)
	}

	if result.PackageName != "multi" {
		t.Errorf("PackageName: want %q, got %q", "multi", result.PackageName)
	}

	if len(result.Services) != 2 {
		t.Errorf("Services count: want 2, got %d", len(result.Services))
	}

	// 模型中应包含 ProductModel（有 gorm tag）
	var hasProductModel bool
	for _, m := range result.Models {
		if m.Name == "ProductModel" {
			hasProductModel = true
		}
	}
	if !hasProductModel {
		t.Error("Models should contain ProductModel")
	}
}

func TestParseFull_NoService(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("noservice.go"))
	if err != nil {
		t.Fatalf("ParseFull: unexpected error: %v", err)
	}
	if len(result.Services) != 0 {
		t.Errorf("Services: want 0, got %d", len(result.Services))
	}
	if len(result.Models) == 0 {
		t.Error("Models should not be empty for noservice.go")
	}
}

func TestParseFull_FileNotFound(t *testing.T) {
	_, err := parser.ParseFull(testdataPath("nonexistent.go"))
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestParseFull_UsersvcIDL(t *testing.T) {
	// 使用项目自带的 usersvc IDL
	idlPath := filepath.Join("..", "..", "..", "examples", "usersvc", "idl.go")
	result, err := parser.ParseFull(idlPath)
	if err != nil {
		t.Fatalf("ParseFull usersvc: %v", err)
	}
	if result.PackageName != "usersvc" {
		t.Errorf("PackageName: want %q, got %q", "usersvc", result.PackageName)
	}
	if len(result.Services) != 1 {
		t.Fatalf("Services: want 1, got %d", len(result.Services))
	}
	svc := result.Services[0]
	if svc.ServiceName != "UserService" {
		t.Errorf("ServiceName: want %q, got %q", "UserService", svc.ServiceName)
	}
	if len(svc.Methods) != 5 {
		t.Errorf("Methods: want 5, got %d", len(svc.Methods))
	}
}

// ─────────────────────────── Parse（向后兼容入口）───────────────────────────

func TestParse_BackwardCompatibility(t *testing.T) {
	pkgName, services, err := parser.Parse(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	if pkgName != "basic" {
		t.Errorf("PackageName: want %q, got %q", "basic", pkgName)
	}
	if len(services) == 0 {
		t.Error("services should not be empty")
	}
}

// ─────────────────────────── Service 结构体字段 ───────────────────────────

func TestService_Fields(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]

	if svc.ServiceName != "UserService" {
		t.Errorf("ServiceName: want %q, got %q", "UserService", svc.ServiceName)
	}
	// PackageName 是 ServiceName 的小写形式
	if svc.PackageName != "userservice" {
		t.Errorf("PackageName: want %q, got %q", "userservice", svc.PackageName)
	}
	// Title 默认 = ServiceName + " API"
	if svc.Title != "UserService API" {
		t.Errorf("Title: want %q, got %q", "UserService API", svc.Title)
	}
}

// ─────────────────────────── Method 字段 ───────────────────────────

func TestMethod_InputOutput(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]

	// 找 CreateUser 方法
	var createMethod *parser.Method
	for i := range svc.Methods {
		if svc.Methods[i].Name == "CreateUser" {
			createMethod = &svc.Methods[i]
			break
		}
	}
	if createMethod == nil {
		t.Fatal("CreateUser method not found")
	}
	if createMethod.Input != "CreateUserRequest" {
		t.Errorf("Input: want %q, got %q", "CreateUserRequest", createMethod.Input)
	}
	if createMethod.Output != "CreateUserResponse" {
		t.Errorf("Output: want %q, got %q", "CreateUserResponse", createMethod.Output)
	}
}

func TestMethod_DocAndSummary(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]

	for _, m := range svc.Methods {
		if m.Name == "CreateUser" {
			// 注释存在时 Summary 非空
			if m.Summary == "" {
				t.Error("Summary should not be empty for documented method")
			}
			if m.Doc == "" {
				t.Error("Doc should not be empty for documented method")
			}
			return
		}
	}
	t.Error("CreateUser not found")
}

func TestMethod_Tags(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]
	for _, m := range svc.Methods {
		if m.Tags != svc.ServiceName {
			t.Errorf("method %s: Tags want %q, got %q", m.Name, svc.ServiceName, m.Tags)
		}
	}
}

// ─────────────────────────── HTTPMethod 推导 ───────────────────────────

func TestMethod_HTTPMethodInference(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]

	cases := map[string]string{
		"CreateUser":    "post",
		"GetUser":       "get",
		"ListUsers":     "get",
		"FindByEmail":   "get",
		"SearchUsers":   "get",
		"QueryStats":    "get",
		"DeleteUser":    "delete",
		"RemoveExpired": "delete",
		"UpdateUser":    "put",
		"EditProfile":   "put",
		"ModifyEmail":   "put",
		"PatchStatus":   "put",
	}

	methodMap := make(map[string]parser.Method)
	for _, m := range svc.Methods {
		methodMap[m.Name] = m
	}

	for methodName, wantHTTP := range cases {
		m, ok := methodMap[methodName]
		if !ok {
			t.Errorf("method %q not found in service", methodName)
			continue
		}
		if m.HTTPMethod != wantHTTP {
			t.Errorf("method %s: HTTPMethod want %q, got %q", methodName, wantHTTP, m.HTTPMethod)
		}
	}
}

func TestMethod_Route(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]
	for _, m := range svc.Methods {
		expectedRoute := "/" + strings.ToLower(m.Name)
		if m.Route != expectedRoute {
			t.Errorf("method %s: Route want %q, got %q", m.Name, expectedRoute, m.Route)
		}
	}
}

// ─────────────────────────── 无效签名容错 ───────────────────────────

func TestParseFull_InvalidSignatures_SkipsBadMethods(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("invalid.go"))
	if err != nil {
		t.Fatalf("ParseFull: unexpected error: %v", err)
	}
	if len(result.Services) != 1 {
		t.Fatalf("Services: want 1, got %d", len(result.Services))
	}
	svc := result.Services[0]

	// 只有 ValidMethod 能通过校验
	if len(svc.Methods) != 1 {
		t.Errorf("Methods: want 1 (only ValidMethod), got %d (%v)",
			len(svc.Methods), methodNames(svc.Methods))
	}
	if len(svc.Methods) > 0 && svc.Methods[0].Name != "ValidMethod" {
		t.Errorf("surviving method: want %q, got %q", "ValidMethod", svc.Methods[0].Name)
	}
}

// ─────────────────────────── Model 解析 ───────────────────────────

func TestModel_GormTags(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	var userModel *parser.Model
	for _, m := range result.Models {
		if m.Name == "User" {
			userModel = m
			break
		}
	}
	if userModel == nil {
		t.Fatal("User model not found")
	}

	if !userModel.HasGormTags {
		t.Error("User model should have gorm tags")
	}

	// 检查主键字段
	var idField *parser.ModelField
	for i := range userModel.Fields {
		if userModel.Fields[i].Name == "ID" {
			idField = &userModel.Fields[i]
			break
		}
	}
	if idField == nil {
		t.Fatal("ID field not found in User model")
	}
	if !idField.IsPrimary {
		t.Error("ID field should be primary key")
	}
	if !idField.IsAutoIncr {
		t.Error("ID field should be auto increment")
	}

	// Username：not null + unique
	for _, f := range userModel.Fields {
		if f.Name == "Username" {
			if !f.IsNotNull {
				t.Error("Username.IsNotNull should be true")
			}
			if !f.IsUnique {
				t.Error("Username.IsUnique should be true")
			}
		}
	}
}

func TestModel_TableName(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	// User → user
	for _, m := range result.Models {
		if m.Name == "User" {
			if m.TableName != "user" {
				t.Errorf("User.TableName: want %q, got %q", "user", m.TableName)
			}
		}
	}
}

func TestModel_HasGormTags_False(t *testing.T) {
	// noservice.go 的 HelperStruct 没有 gorm tag
	result, err := parser.ParseFull(testdataPath("noservice.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	for _, m := range result.Models {
		if m.Name == "HelperStruct" {
			if m.HasGormTags {
				t.Error("HelperStruct should not have gorm tags")
			}
		}
	}
}

func TestModel_FieldTypes(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	var userModel *parser.Model
	for _, m := range result.Models {
		if m.Name == "User" {
			userModel = m
			break
		}
	}
	if userModel == nil {
		t.Fatal("User model not found")
	}

	wantTypes := map[string]string{
		"ID":       "uint",
		"Username": "string",
		"Age":      "int",
		"Score":    "float64",
		"Active":   "bool",
	}
	fieldMap := make(map[string]parser.ModelField)
	for _, f := range userModel.Fields {
		fieldMap[f.Name] = f
	}
	for name, wantType := range wantTypes {
		f, ok := fieldMap[name]
		if !ok {
			t.Errorf("field %q not found", name)
			continue
		}
		if f.Type != wantType {
			t.Errorf("field %s: Type want %q, got %q", name, wantType, f.Type)
		}
	}
}

func TestModel_SliceAndMapFields(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("noservice.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	for _, m := range result.Models {
		if m.Name == "HelperStruct" {
			fieldMap := make(map[string]parser.ModelField)
			for _, f := range m.Fields {
				fieldMap[f.Name] = f
			}
			if f, ok := fieldMap["Tags"]; ok {
				if f.Type != "[]string" {
					t.Errorf("Tags: want %q, got %q", "[]string", f.Type)
				}
			}
			if f, ok := fieldMap["Options"]; ok {
				if f.Type != "map[string]string" {
					t.Errorf("Options: want %q, got %q", "map[string]string", f.Type)
				}
			}
			if f, ok := fieldMap["Ptr"]; ok {
				if f.Type != "*OnlyModel" {
					t.Errorf("Ptr: want %q, got %q", "*OnlyModel", f.Type)
				}
			}
			return
		}
	}
	t.Error("HelperStruct not found")
}

// ─────────────────────────── SwagType 映射 ───────────────────────────

func TestModel_SwagTypes(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	var userModel *parser.Model
	for _, m := range result.Models {
		if m.Name == "User" {
			userModel = m
			break
		}
	}
	if userModel == nil {
		t.Fatal("User model not found")
	}

	wantSwagTypes := map[string]string{
		"ID":       "integer",
		"Username": "string",
		"Age":      "integer",
		"Score":    "number",
		"Active":   "boolean",
	}
	fieldMap := make(map[string]parser.ModelField)
	for _, f := range userModel.Fields {
		fieldMap[f.Name] = f
	}
	for name, wantST := range wantSwagTypes {
		f, ok := fieldMap[name]
		if !ok {
			t.Errorf("field %q not found", name)
			continue
		}
		if f.SwagType != wantST {
			t.Errorf("field %s: SwagType want %q, got %q", name, wantST, f.SwagType)
		}
	}
}

// ─────────────────────────── JSONTag 解析 ───────────────────────────

func TestModel_JSONTag(t *testing.T) {
	result, err := parser.ParseFull(testdataPath("basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	for _, m := range result.Models {
		if m.Name == "User" {
			for _, f := range m.Fields {
				if f.Name == "Username" && f.JSONTag != "username" {
					t.Errorf("Username JSONTag: want %q, got %q", "username", f.JSONTag)
				}
				if f.Name == "ID" && f.JSONTag != "id" {
					t.Errorf("ID JSONTag: want %q, got %q", "id", f.JSONTag)
				}
			}
			return
		}
	}
}

// ─────────────────────────── 辅助函数 ───────────────────────────

func TestToSnakeCase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"User", "user"},
		{"UserProfile", "user_profile"},
		{"UserID", "user_id"},
		{"ID", "id"},
		{"HTTPClient", "http_client"},
	}
	for _, c := range cases {
		got := parser.ToSnakeCase(c.in)
		if got != c.want {
			t.Errorf("ToSnakeCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func methodNames(methods []parser.Method) []string {
	names := make([]string, len(methods))
	for i, m := range methods {
		names[i] = m.Name
	}
	return names
}
