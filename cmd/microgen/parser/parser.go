package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
)

// ─────────────────────────── 公共数据结构 ───────────────────────────

// Service 服务定义（接口）
type Service struct {
	ServiceName string
	PackageName string
	Methods     []Method
	Title       string // swag @title（默认 = ServiceName + " API"）
	Description string // swag @description（取接口注释）
}

// Method 服务方法
type Method struct {
	Name       string
	Input      string
	Output     string
	Doc        string // 方法注释（原始文本）
	Summary    string // swag @Summary（取 Doc 第一行）
	Tags       string // swag @Tags（默认 = ServiceName）
	HTTPMethod string // swag @Router 方法（默认 POST）
	Route      string // swag @Router 路径（默认 /{lower(Name)}）
}

// ModelField 模型字段（结构体字段）
type ModelField struct {
	Name       string // Go 字段名
	Type       string // Go 类型
	JSONTag    string // json tag 值
	GormTag    string // gorm tag 值
	Comment    string // 行注释
	IsPrimary  bool   // 是否主键
	IsAutoIncr bool   // 是否自增
	IsNotNull  bool   // 是否非空
	IsUnique   bool   // 是否唯一索引
	SwagType   string // swag 类型（integer/string/number/boolean/array/object）
	Example    string // swag 示例值
}

// Model 从 IDL 解析出的 gorm Model 定义
type Model struct {
	Name        string       // struct 名称
	TableName   string       // 数据库表名（snake_case，可被 gorm tag 覆盖）
	Comment     string       // struct 注释
	Fields      []ModelField // 所有字段
	HasGormTags bool         // 是否含 gorm tag（用于判断是否需要生成 model 文件）
}

// ParseResult 完整解析结果
type ParseResult struct {
	PackageName string
	Services    []*Service
	Models      []*Model
}

// ─────────────────────────── 公共入口 ───────────────────────────

// Parse 解析IDL文件生成服务定义（向后兼容）
func Parse(idlPath string) (packageName string, services []*Service, err error) {
	result, err := ParseFull(idlPath)
	if err != nil {
		return "", nil, err
	}
	return result.PackageName, result.Services, nil
}

// ParseFull 解析IDL文件，同时返回服务定义和模型定义
func ParseFull(idlPath string) (*ParseResult, error) {
	absPath, err := filepath.Abs(idlPath)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	result := &ParseResult{
		PackageName: file.Name.Name,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		switch t := ts.Type.(type) {
		case *ast.InterfaceType:
			// 解析服务接口
			svc := parseService(ts, t)
			result.Services = append(result.Services, svc)

		case *ast.StructType:
			// 解析结构体 → 可能是 gorm model
			model := parseModel(ts, t)
			result.Models = append(result.Models, model)
		}

		return true
	})

	return result, nil
}

// ─────────────────────────── 接口解析 ───────────────────────────

func parseService(ts *ast.TypeSpec, iface *ast.InterfaceType) *Service {
	svc := &Service{
		ServiceName: ts.Name.Name,
		PackageName: strings.ToLower(ts.Name.Name),
	}

	// 接口注释 → Title / Description
	svc.Title = ts.Name.Name + " API"
	if ts.Comment != nil {
		svc.Description = strings.TrimSpace(ts.Comment.Text())
	}

	for _, m := range iface.Methods.List {
		method, err := parseMethod(m, svc.ServiceName)
		if err != nil {
			fmt.Printf("Warning: failed to parse method: %v\n", err)
			continue
		}
		svc.Methods = append(svc.Methods, method)
	}
	return svc
}

// parseMethod 解析接口方法
func parseMethod(field *ast.Field, serviceName string) (Method, error) {
	if len(field.Names) == 0 {
		return Method{}, fmt.Errorf("method has no name")
	}

	method := Method{
		Name: field.Names[0].Name,
	}

	// 注释 → Doc & Summary
	if field.Doc != nil {
		raw := strings.TrimSpace(field.Doc.Text())
		method.Doc = raw
		// Summary 取第一行（去掉可能的方法名前缀）
		firstLine := strings.SplitN(raw, "\n", 2)[0]
		method.Summary = firstLine
	}
	if method.Summary == "" {
		method.Summary = method.Name
	}

	// Tags 默认使用服务名
	method.Tags = serviceName

	// HTTPMethod / Route 根据方法名前缀推导
	name := strings.ToLower(method.Name)
	switch {
	case strings.HasPrefix(name, "get") || strings.HasPrefix(name, "find") ||
		strings.HasPrefix(name, "query") || strings.HasPrefix(name, "list") ||
		strings.HasPrefix(name, "search"):
		method.HTTPMethod = "get"
	case strings.HasPrefix(name, "delete") || strings.HasPrefix(name, "remove"):
		method.HTTPMethod = "delete"
	case strings.HasPrefix(name, "update") || strings.HasPrefix(name, "edit") ||
		strings.HasPrefix(name, "modify") || strings.HasPrefix(name, "patch"):
		method.HTTPMethod = "put"
	default:
		method.HTTPMethod = "post"
	}
	method.Route = "/" + name

	funcType, ok := field.Type.(*ast.FuncType)
	if !ok {
		return method, fmt.Errorf("method %s has invalid type", method.Name)
	}

	if err := validateMethodSignature(funcType); err != nil {
		return method, fmt.Errorf("invalid method signature for %s: %v", method.Name, err)
	}

	// 解析输入参数（跳过 context.Context）
	for _, param := range funcType.Params.List {
		if isContextType(param.Type) {
			continue
		}
		inputType, err := getTypeName(param.Type)
		if err != nil {
			return method, fmt.Errorf("invalid input type for method %s: %v", method.Name, err)
		}
		method.Input = inputType
	}

	// 解析返回值（取第一个非 error 值）
	for _, param := range funcType.Results.List {
		if isContextType(param.Type) {
			continue
		}
		outputType, err := getTypeName(param.Type)
		if err != nil {
			return method, fmt.Errorf("invalid output type for method %s: %v", method.Name, err)
		}
		method.Output = outputType
		break
	}

	return method, nil
}

func validateMethodSignature(funcType *ast.FuncType) error {
	if len(funcType.Params.List) < 2 {
		return fmt.Errorf("method must have at least context.Context and request parameter")
	}
	if !isContextType(funcType.Params.List[0].Type) {
		return fmt.Errorf("first parameter must be context.Context")
	}
	if len(funcType.Results.List) != 2 {
		return fmt.Errorf("method must return exactly 2 values: response and error")
	}
	if !isErrorType(funcType.Results.List[1].Type) {
		return fmt.Errorf("last return value must be error")
	}
	return nil
}

// ─────────────────────────── 结构体/Model 解析 ───────────────────────────

func parseModel(ts *ast.TypeSpec, st *ast.StructType) *Model {
	model := &Model{
		Name:      ts.Name.Name,
		TableName: toSnakeCase(ts.Name.Name),
	}

	if ts.Comment != nil {
		model.Comment = strings.TrimSpace(ts.Comment.Text())
	}

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// 嵌入字段，跳过
			continue
		}

		mf := ModelField{
			Name: field.Names[0].Name,
		}

		// 字段类型
		typeName, _ := getTypeName(field.Type)
		mf.Type = typeName
		mf.SwagType = goTypeToSwagType(typeName)
		mf.Example = swagExample(typeName, mf.JSONTag)

		// 行注释
		if field.Comment != nil {
			mf.Comment = strings.TrimSpace(field.Comment.Text())
		}

		// 解析 tag
		if field.Tag != nil {
			rawTag := strings.Trim(field.Tag.Value, "`")
			st := reflect.StructTag(rawTag)

			mf.JSONTag = st.Get("json")
			mf.GormTag = st.Get("gorm")

			if mf.GormTag != "" {
				model.HasGormTags = true
				mf.IsPrimary = strings.Contains(mf.GormTag, "primaryKey") ||
					strings.Contains(mf.GormTag, "primary_key")
				mf.IsAutoIncr = strings.Contains(mf.GormTag, "autoIncrement") ||
					strings.Contains(mf.GormTag, "auto_increment")
				mf.IsNotNull = strings.Contains(mf.GormTag, "not null")
				mf.IsUnique = strings.Contains(mf.GormTag, "unique")

				// 从 gorm tag 中提取 column 覆盖
				// gorm:"column:xxx" → 不影响 Model.TableName，只是字段级
			}
		}

		model.Fields = append(model.Fields, mf)
	}

	return model
}

// ─────────────────────────── 辅助函数 ───────────────────────────

func isContextType(expr ast.Expr) bool {
	selExpr, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := selExpr.X.(*ast.Ident)
	return ok && ident.Name == "context" && selExpr.Sel.Name == "Context"
}

func isErrorType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}

func getTypeName(expr ast.Expr) (string, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name, nil
	case *ast.StarExpr:
		name, err := getTypeName(t.X)
		return "*" + name, err
	case *ast.SelectorExpr:
		xName, err := getTypeName(t.X)
		if err != nil {
			return "", err
		}
		return xName + "." + t.Sel.Name, nil
	case *ast.ArrayType:
		elemName, err := getTypeName(t.Elt)
		if err != nil {
			return "", err
		}
		return "[]" + elemName, nil
	case *ast.MapType:
		keyName, err := getTypeName(t.Key)
		if err != nil {
			return "", err
		}
		valName, err := getTypeName(t.Value)
		if err != nil {
			return "", err
		}
		return "map[" + keyName + "]" + valName, nil
	default:
		return "", fmt.Errorf("unsupported type: %T", expr)
	}
}

// toSnakeCase 将 CamelCase 转换为 snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(r + 32)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// goTypeToSwagType 将 Go 类型映射为 swaggo 类型字符串
func goTypeToSwagType(goType string) string {
	// 去掉指针前缀
	t := strings.TrimPrefix(goType, "*")
	switch t {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "bool":
		return "boolean"
	case "string":
		return "string"
	default:
		if strings.HasPrefix(t, "[]") {
			return "array"
		}
		if strings.HasPrefix(t, "map[") {
			return "object"
		}
		return "object"
	}
}

// swagExample 返回字段的 swag 示例值（用于注释可读性，非强制）
func swagExample(goType, jsonTag string) string {
	// 用 json tag 的首段作为字段名提示
	name := strings.Split(jsonTag, ",")[0]
	t := strings.TrimPrefix(goType, "*")
	switch {
	case t == "string":
		if name != "" {
			return `"` + name + `"`
		}
		return `"example"`
	case t == "bool":
		return "true"
	case strings.HasPrefix(t, "int") || strings.HasPrefix(t, "uint"):
		return "1"
	case strings.HasPrefix(t, "float"):
		return "1.0"
	default:
		return "{}"
	}
}
