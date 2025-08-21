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
func (g *Generator) Generate(service *parser.Service) error {
	// 创建项目目录结构
	if err := g.createDirStructure(service); err != nil {
		return err
	}

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

	// 生成主程序文件
	return g.generateMainFile(service)
}

// 创建目录结构 - 根据服务名动态生成
func (g *Generator) createDirStructure(service *parser.Service) error {
	// 确定服务名称
	serviceName := service.ServiceName
	if g.config.ServiceName != "" {
		serviceName = g.config.ServiceName
	}

	// 创建cmd目录
	cmdDir := filepath.Join(g.outputDir, "cmd", serviceName)
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		return fmt.Errorf("failed to create cmd directory: %v", err)
	}

	// 创建client目录
	clientDir := filepath.Join(g.outputDir, "client")
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		return fmt.Errorf("failed to create client directory: %v", err)
	}

	// 创建api目录存放接口定义
	apiDir := filepath.Join(g.outputDir, "api")
	return os.MkdirAll(apiDir, 0755)
}

// 生成服务实现文件
func (g *Generator) generateServiceFile(service *parser.Service) error {
	// 准备模板数据
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}

	filePath := filepath.Join(g.outputDir, "service.go")
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
	filePath := filepath.Join(g.outputDir, "endpoints.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create endpoints file: %v", err)
	}
	defer f.Close()

	// 执行模板
	if err := g.templates.ExecuteTemplate(f, "endpoints.tmpl", service); err != nil {
		return fmt.Errorf("failed to execute endpoints template: %v", err)
	}
	return nil
}

// 生成传输层文件
func (g *Generator) generateTransportFile(service *parser.Service) error {
	filePath := filepath.Join(g.outputDir, "transport.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create transport file: %v", err)
	}
	defer f.Close()

	// 执行模板
	if err := g.templates.ExecuteTemplate(f, "transport.tmpl", service); err != nil {
		return fmt.Errorf("failed to execute transport template: %v", err)
	}
	return nil
}

// 生成主程序文件
func (g *Generator) generateMainFile(service *parser.Service) error {
	// 确定服务名称
	serviceName := service.ServiceName
	if g.config.ServiceName != "" {
		serviceName = g.config.ServiceName
	}

	filePath := filepath.Join(g.outputDir, "cmd", serviceName, "main.go")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create main file: %v", err)
	}
	defer f.Close()

	// 执行模板
	return g.templates.ExecuteTemplate(f, "main.tmpl", service)
}
