package generator

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// Options 生成器配置选项
type Options struct {
	TemplateFS  *embed.FS // 模板文件系统
	OutputDir   string    // 输出目录
	ImportPath  string    // 项目导入路径
	ServiceName string    // 服务名称 (可选，默认从IDL获取)
	Protocols   []string  // 支持的协议 (http, grpc等)
	WithConfig  bool      // 是否生成配置文件
	WithDocs    bool      // 是否生成文档
	WithTests   bool      // 是否生成测试文件
}

// Generator 代码生成器
type Generator struct {
	outputDir string
	templates *template.Template
	config    Options
}

// New 创建代码生成器实例
func New(opts Options) (*Generator, error) {
	// 从嵌入文件系统解析模板
	filenames, err := fs.Glob(opts.TemplateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to find template files: %v", err)
	}

	// 创建自定义函数映射
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}

	// 解析模板文件
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(opts.TemplateFS, filenames...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %v", err)
	}

	return &Generator{
		outputDir: opts.OutputDir,
		templates: tmpl,
		config:    opts,
	}, nil
}

// Generate 根据服务定义生成代码
func (g *Generator) Generate(services []*parser.Service) error {
	// 创建项目目录结构
	if err := g.createDirStructure(); err != nil {
		return err
	}

	// 为每个服务生成代码文件
	for _, service := range services {
		// 生成服务代码文件
		if err := g.generateServiceFile(service); err != nil {
			return err
		}

		// 生成端点代码文件
		if err := g.generateEndpointsFile(service); err != nil {
			return err
		}

		// 生成传输层代码文件
		if err := g.generateTransportFile(service); err != nil {
			return err
		}

		// 生成测试文件
		if g.config.WithTests {
			if err := g.generateTestFile(service); err != nil {
				return err
			}
		}
	}

	// 生成主程序文件
	if err := g.generateMainFile(services); err != nil {
		return err
	}

	// 生成配置文件
	if g.config.WithConfig {
		if err := g.generateConfigFile(services); err != nil {
			return err
		}
	}

	// 生成文档
	if g.config.WithDocs {
		if err := g.generateReadme(services); err != nil {
			return err
		}
	}

	return nil
}

// 创建目录结构
func (g *Generator) createDirStructure() error {
	dirs := []string{
		filepath.Join(g.outputDir, "cmd"),
		filepath.Join(g.outputDir, "client"),
		filepath.Join(g.outputDir, "api"),
		filepath.Join(g.outputDir, "service"),
		filepath.Join(g.outputDir, "endpoint"),
		filepath.Join(g.outputDir, "transport"),
		filepath.Join(g.outputDir, "config"),
		filepath.Join(g.outputDir, "docs"),
		filepath.Join(g.outputDir, "test"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	return nil
}

// 生成主程序文件
func (g *Generator) generateMainFile(services []*parser.Service) error {
	data := map[string]interface{}{
		"Services":   services,
		"ImportPath": g.config.ImportPath,
		"NeedMux":    len(services) > 1,
	}

	filePath := filepath.Join(g.outputDir, "cmd", "main.go")
	return g.executeTemplate("main.tmpl", filePath, data)
}

// 生成服务文件
func (g *Generator) generateServiceFile(service *parser.Service) error {
	serviceDir := filepath.Join(g.outputDir, "service", service.PackageName)
	os.MkdirAll(serviceDir, 0755)

	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}

	filePath := filepath.Join(serviceDir, "service.go")
	return g.executeTemplate("service.tmpl", filePath, data)
}

// 生成端点文件
func (g *Generator) generateEndpointsFile(service *parser.Service) error {
	endpointDir := filepath.Join(g.outputDir, "endpoint", service.PackageName)
	os.MkdirAll(endpointDir, 0755)

	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}

	filePath := filepath.Join(endpointDir, "endpoints.go")
	return g.executeTemplate("endpoints.tmpl", filePath, data)
}

// 生成传输层文件
func (g *Generator) generateTransportFile(service *parser.Service) error {
	transportDir := filepath.Join(g.outputDir, "transport", service.PackageName)
	os.MkdirAll(transportDir, 0755)
	data := map[string]interface{}{
		"Service":     service,
		"ImportPath":  g.config.ImportPath,
		"RoutePrefix": service.PackageName,
	}

	filePath := filepath.Join(transportDir, "transport.go")
	return g.executeTemplate("transport.tmpl", filePath, data)
}

// 生成测试文件
func (g *Generator) generateTestFile(service *parser.Service) error {
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}

	filePath := filepath.Join(g.outputDir, "test", service.PackageName+"_test.go")

	testTemplate := `package test

import (
	"context"
	"testing"

	"{{.ImportPath}}/service/{{.Service.PackageName}}"
	idl "{{.ImportPath}}"
)

func Test{{.Service.ServiceName}}(t *testing.T) {
	svc := {{.Service.PackageName}}.NewService(nil)
	ctx := context.Background()

{{range .Service.Methods}}
	t.Run("{{.Name}}", func(t *testing.T) {
		req := idl.{{.Input}}{}
		resp, err := svc.{{.Name}}(ctx, req)
		if err != nil {
			t.Errorf("{{.Name}} failed: %v", err)
		}
		if resp == nil {
			t.Error("{{.Name}} returned nil response")
		}
	})
{{end}}
}
`

	tmpl, err := template.New("test").Parse(testTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// 生成配置文件
func (g *Generator) generateConfigFile(services []*parser.Service) error {
	data := map[string]interface{}{
		"ServiceName": services[0].ServiceName,
	}

	filePath := filepath.Join(g.outputDir, "config", "config.yaml")

	configTemplate := `# {{.ServiceName}} 服务配置
server:
  port: 8080
  timeout: 30s

logging:
  level: "info"

database:
  host: "localhost"
  port: 5432
`
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// 生成文档
func (g *Generator) generateReadme(services []*parser.Service) error {
	data := map[string]interface{}{
		"Services": services,
	}

	filePath := filepath.Join(g.outputDir, "README.md")

	// 创建自定义函数映射
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
	}

	readmeTemplate := `# {{range .Services}}{{.ServiceName}}{{end}} 微服务

## API 端点
{{range .Services}}{{range .Methods}}
- **POST** /{{$.Service.PackageName}}/{{.Name | lower}}
{{end}}{{end}}
`
	tmpl, err := template.New("readme").Funcs(funcMap).Parse(readmeTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// 执行模板
func (g *Generator) executeTemplate(templateName, filePath string, data interface{}) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filePath, err)
	}
	defer f.Close()

	return g.templates.ExecuteTemplate(f, templateName, data)
}
