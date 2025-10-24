package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// 配置参数结构体
type config struct {
	idlPath     string
	outputDir   string
	ImportPath  string
	protocols   []string
	withConfig  bool
	withDocs    bool
	withTests   bool
	serviceName string
}

// 解析命令行参数
func parseFlags() config {
	idlPath := flag.String("idl", "", "Path to IDL file")
	outputDir := flag.String("out", ".", "Output directory")
	importPath := flag.String("import", "", "Import path")
	protocols := flag.String("protocols", "http", "Supported protocols (comma separated: http,grpc)")
	withConfig := flag.Bool("config", true, "Generate configuration files")
	withDocs := flag.Bool("docs", true, "Generate documentation")
	withTests := flag.Bool("tests", false, "Generate test files")
	serviceName := flag.String("service", "", "Custom service name (default from IDL)")

	flag.Parse()

	return config{
		idlPath:     *idlPath,
		outputDir:   *outputDir,
		ImportPath:  *importPath,
		protocols:   strings.Split(*protocols, ","),
		withConfig:  *withConfig,
		withDocs:    *withDocs,
		withTests:   *withTests,
		serviceName: *serviceName,
	}
}

// 验证配置参数
func (c config) validate() error {
	if c.idlPath == "" {
		return fmt.Errorf("IDL file path is required")
	}

	// 检查文件是否存在
	if _, err := os.Stat(c.idlPath); os.IsNotExist(err) {
		return fmt.Errorf("IDL file not found: %s", c.idlPath)
	}

	// 验证协议支持
	for _, protocol := range c.protocols {
		if protocol != "http" && protocol != "grpc" {
			return fmt.Errorf("unsupported protocol: %s", protocol)
		}
	}

	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 解析并验证参数
	cfg := parseFlags()
	if err := cfg.validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// 解析IDL文件
	log.Printf("Parsing IDL file: %s", cfg.idlPath)
	packageName, services, err := parser.Parse(cfg.idlPath)
	if err != nil {
		log.Fatalf("Failed to parse IDL: %v", err)
	}

	// 如果没有指定服务名，使用第一个服务的名称
	if cfg.serviceName == "" && len(services) > 0 {
		cfg.serviceName = services[0].ServiceName
	}

	// 初始化生成器
	log.Printf("Initializing code generator for package: %s", packageName)
	gen, err := generator.New(generator.Options{
		TemplateFS:  &templateFS,
		OutputDir:   cfg.outputDir,
		ImportPath:  cfg.ImportPath,
		ServiceName: cfg.serviceName,
		Protocols:   cfg.protocols,
		WithConfig:  cfg.withConfig,
		WithDocs:    cfg.withDocs,
		WithTests:   cfg.withTests,
	})
	if err != nil {
		log.Fatalf("Failed to create generator: %v", err)
	}

	// 生成代码
	log.Println("Starting code generation...")
	if err := gen.Generate(services); err != nil {
		log.Fatalf("Code generation failed: %v", err)
	}

	log.Printf("Code generated successfully in: %s", cfg.outputDir)
	log.Printf("Generated %d service(s)", len(services))
	for _, service := range services {
		log.Printf("  - %s with %d method(s)", service.ServiceName, len(service.Methods))
	}
}
