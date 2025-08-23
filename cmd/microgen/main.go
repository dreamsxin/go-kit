package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// 配置参数结构体
type config struct {
	idlPath    string
	outputDir  string
	ImportPath string
}

// 解析命令行参数
func parseFlags() config {
	idlPath := flag.String("idl", "", "Path to IDL file")
	outputDir := flag.String("out", ".", "Output directory")
	importPath := flag.String("import", "", "Import path")

	flag.Parse()

	return config{
		idlPath:    *idlPath,
		outputDir:  *outputDir,
		ImportPath: *importPath,
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
	ast, err := parser.Parse(cfg.idlPath)
	if err != nil {
		log.Fatalf("Failed to parse IDL: %v", err)
	}
	// 初始化生成器
	log.Printf("Initializing code generator for package: %s", ast.PackageName)
	gen, err := generator.New(generator.Options{
		TemplateFS: &templateFS,
		OutputDir:  cfg.outputDir,
		ImportPath: cfg.ImportPath,
	})
	if err != nil {
		log.Fatalf("Failed to create generator: %v", err)
	}

	// 生成代码
	log.Println("Starting code generation...")
	if err := gen.Generate(ast); err != nil {
		log.Fatalf("Code generation failed: %v", err)
	}

	log.Println("Code generated successfully!")
}
