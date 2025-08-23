package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// AST结构定义
type Service struct {
	ServiceName string
	PackageName string
	Methods     []Method
}

type Method struct {
	Name   string
	Input  string
	Output string
}

// Parse 解析IDL文件生成服务定义
func Parse(idlPath string) (packageName string, services []*Service, err error) {
	absPath, err := filepath.Abs(idlPath)
	if err != nil {
		return "", nil, err
	}

	// 读取文件内容
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return "", nil, err
	}

	// 提取 IDL 文件中的包名
	packageName = file.Name.Name

	fmt.Println("Package Name:", packageName)

	// 提取所有服务接口定义
	services = []*Service{}
	ast.Inspect(file, func(n ast.Node) bool {
		// 查找所有接口定义
		if ts, ok := n.(*ast.TypeSpec); ok {
			if iface, ok := ts.Type.(*ast.InterfaceType); ok {
				service := &Service{
					ServiceName: ts.Name.Name,
					PackageName: strings.ToLower(ts.Name.Name),
				}

				// 解析接口方法
				for _, m := range iface.Methods.List {
					method, err := parseMethod(m)
					if err != nil {
						// 记录警告但继续解析其他方法
						fmt.Printf("Warning: failed to parse method: %v\n", err)
						continue
					}
					service.Methods = append(service.Methods, method)
				}

				services = append(services, service)
			}
		}
		return true
	})

	return packageName, services, nil
}

// 解析方法定义
func parseMethod(field *ast.Field) (Method, error) {
	if len(field.Names) == 0 {
		return Method{}, fmt.Errorf("method has no name")
	}

	method := Method{
		Name: field.Names[0].Name,
	}

	// 解析函数签名
	funcType, ok := field.Type.(*ast.FuncType)
	if !ok {
		return method, fmt.Errorf("method %s has invalid type", method.Name)
	}

	// 解析输入参数
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

	// 解析返回值
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

// 辅助函数：判断是否为context.Context类型
func isContextType(expr ast.Expr) bool {
	selExpr, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := selExpr.X.(*ast.Ident)
	return ok && ident.Name == "context" && selExpr.Sel.Name == "Context"
}

// 获取类型名称，支持结构体和指针类型
func getTypeName(expr ast.Expr) (string, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name, nil
	case *ast.StarExpr:
		// 处理指针类型
		name, err := getTypeName(t.X)
		return "*" + name, err
	case *ast.SelectorExpr:
		// 处理包限定类型
		xName, err := getTypeName(t.X)
		if err != nil {
			return "", err
		}
		return xName + "." + t.Sel.Name, nil
	default:
		return "", fmt.Errorf("unsupported type: %T", expr)
	}
}
