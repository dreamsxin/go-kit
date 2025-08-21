package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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
func Parse(idlPath string) (*Service, error) {
	absPath, err := filepath.Abs(idlPath)
	if err != nil {
		return nil, err
	}

	// 读取文件内容
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// 提取 IDL 文件中的包名
	packageName := file.Name.Name

	fmt.Println("Package Name:", packageName)

	// 提取服务接口定义
	service := &Service{
		PackageName: packageName,
	}
	ast.Inspect(file, func(n ast.Node) bool {
		// 查找接口定义
		if iface, ok := n.(*ast.InterfaceType); ok {
			// 获取接口名称
			if ident, ok := n.(*ast.Ident); ok {
				service.ServiceName = ident.Name
			}

			// 解析接口方法
			for _, m := range iface.Methods.List {
				method, err := parseMethod(m)
				if err != nil {
					// 可以考虑在这里记录警告日志，而不是直接返回错误
					// 以便继续解析其他方法
					continue
				}
				service.Methods = append(service.Methods, method)
			}
		}
		return true
	})

	return service, nil
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
	if funcType.Params.List != nil && len(funcType.Params.List) > 0 {
		inputType, err := getTypeName(funcType.Params.List[0].Type)
		if err != nil {
			return method, fmt.Errorf("invalid input type for method %s: %v", method.Name, err)
		}
		method.Input = inputType
	}

	// 解析返回值
	if funcType.Results.List != nil && len(funcType.Results.List) > 0 {
		outputType, err := getTypeName(funcType.Results.List[0].Type)
		if err != nil {
			return method, fmt.Errorf("invalid output type for method %s: %v", method.Name, err)
		}
		method.Output = outputType
	}

	return method, nil
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
