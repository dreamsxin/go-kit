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
	fmt.Println("Template filenames:", filenames)

	// 创建自定义函数映射
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
	}
	// 解析模板文件
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(opts.TemplateFS, filenames...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %v", err)
	}
	for _, t := range tmpl.Templates() {
		fmt.Println("Template name:", t.Name())
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
	}

	// 生成主程序文件（包含所有服务）
	return g.generateMainFile(services)
}

// 创建目录结构 - 多服务共享结构
func (g *Generator) createDirStructure() error {
	// 创建基础目录
	dirs := []string{
		filepath.Join(g.outputDir, "cmd"),
		filepath.Join(g.outputDir, "client"),
		filepath.Join(g.outputDir, "api"),
		filepath.Join(g.outputDir, "service"), // 服务根目录
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}
	return nil
}

// 生成主程序文件（支持多服务）
func (g *Generator) generateMainFile(services []*parser.Service) error {
	// 准备模板数据
	data := map[string]interface{}{
		"Services":   services,
		"ImportPath": g.config.ImportPath,
		"NeedMux":    len(services) > 1,
	}

	filePath := filepath.Join(g.outputDir, "cmd", "main.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create main file: %v", err)
	}
	defer f.Close()

	// 执行模板
	return g.templates.ExecuteTemplate(f, "main.tmpl", data)
}

// 生成服务实现文件
func (g *Generator) generateServiceFile(service *parser.Service) error {
	// 准备模板数据
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}

	// 创建服务目录
	serviceDir := filepath.Join(g.outputDir, "service", service.PackageName)
	os.MkdirAll(serviceDir, 0755)

	filePath := filepath.Join(serviceDir, "service.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create service file: %v", err)
	}
	defer f.Close()

	// 执行模板
	if err := g.templates.ExecuteTemplate(f, "service.tmpl", data); err != nil {
		return fmt.Errorf("failed to execute service template: %v", err)
	}
	return nil
}

// 生成端点文件
func (g *Generator) generateEndpointsFile(service *parser.Service) error {
	serviceDir := filepath.Join(g.outputDir, "service", service.PackageName)
	filePath := filepath.Join(serviceDir, "endpoints.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create endpoints file: %v", err)
	}
	defer f.Close()

	// 准备模板数据
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}

	// 执行模板
	if err := g.templates.ExecuteTemplate(f, "endpoints.tmpl", data); err != nil {
		return fmt.Errorf("failed to execute endpoints template: %v", err)
	}
	return nil
}

// 生成传输层文件
func (g *Generator) generateTransportFile(service *parser.Service) error {
	serviceDir := filepath.Join(g.outputDir, "service", service.PackageName)
	filePath := filepath.Join(serviceDir, "transport.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create transport file: %v", err)
	}
	defer f.Close()

	// 准备模板数据
	data := map[string]interface{}{
		"Service":     service,
		"ImportPath":  g.config.ImportPath,
		"RoutePrefix": service.PackageName,
	}

	// 执行模板
	if err := g.templates.ExecuteTemplate(f, "transport.tmpl", data); err != nil {
		return fmt.Errorf("failed to execute transport template: %v", err)
	}
	return nil
}
