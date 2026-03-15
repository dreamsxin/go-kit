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

// config 命令行参数配置
type config struct {
	idlPath     string
	outputDir   string
	ImportPath  string
	protocols   []string
	withConfig  bool
	withDocs    bool
	withTests   bool
	withModel   bool   // 是否生成 gorm model + repository
	withGRPC    bool   // 是否生成 gRPC 传输层（可通过 -protocols grpc 隐式开启）
	withDB      bool   // main 是否包含数据库初始化
	dbDriver    string // gorm 数据库驱动: sqlite(默认)/mysql/postgres/sqlserver/clickhouse
	withSwag    bool   // 是否生成 swaggo 注释 + docs stub + /swagger/ 路由
	serviceName string
	routePrefix string // HTTP 路由前缀，留空时使用服务名（如 /userservice）
}

func parseFlags() config {
	idlPath := flag.String("idl", "", "Path to IDL file (required)")
	outputDir := flag.String("out", ".", "Output directory")
	importPath := flag.String("import", "", "Go module import path for the generated project")
	protocols := flag.String("protocols", "http", "Supported protocols, comma-separated: http,grpc")
	withConfig := flag.Bool("config", true, "Generate config/config.yaml")
	withDocs := flag.Bool("docs", true, "Generate README.md")
	withTests := flag.Bool("tests", false, "Generate unit test files")
	withModel := flag.Bool("model", false, "Generate gorm model & repository layer")
	withDB := flag.Bool("db", false, "Include database initialization in main.go")
	dbDriver := flag.String("db.driver", "sqlite", "Gorm database driver: sqlite(default), mysql, postgres, sqlserver, clickhouse")
	withSwag := flag.Bool("swag", false, "Generate swaggo annotations + docs stub + /swagger/ UI route")
	serviceName := flag.String("service", "", "Override service name (default: first interface name in IDL)")
	routePrefix := flag.String("prefix", "", "HTTP route prefix (e.g. /api/v1). Defaults to /<servicename> if empty")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `microgen - Go microservice code generator

Usage:
  microgen -idl <idl_file> -out <output_dir> -import <import_path> [options]

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # 生成 HTTP 服务（默认 sqlite）
  microgen -idl ./examples/usersvc/idl.go -out ./generated -import github.com/example/myapp -model -db

  # 指定 MySQL 驱动
  microgen -idl ./examples/usersvc/idl.go -out ./generated -import github.com/example/myapp -model -db -db.driver mysql

  # HTTP + gRPC + PostgreSQL
  microgen -idl ./examples/usersvc/idl.go -out ./generated -import github.com/example/myapp \
    -protocols http,grpc -model -db -db.driver postgres

  # HTTP + Swagger 文档
  microgen -idl ./examples/usersvc/idl.go -out ./generated -import github.com/example/myapp -swag

  # 指定路由前缀
  microgen -idl ./examples/usersvc/idl.go -out ./generated -import github.com/example/myapp -prefix /api/v1

  # 完整版（HTTP + gRPC + model + swagger）
  microgen -idl ./examples/usersvc/idl.go -out ./generated -import github.com/example/myapp \
    -protocols http,grpc -model -db -swag
`)
	}

	flag.Parse()

	protos := strings.Split(*protocols, ",")
	hasGRPC := false
	for _, p := range protos {
		if strings.TrimSpace(p) == "grpc" {
			hasGRPC = true
		}
	}

	return config{
		idlPath:     *idlPath,
		outputDir:   *outputDir,
		ImportPath:  *importPath,
		protocols:   protos,
		withConfig:  *withConfig,
		withDocs:    *withDocs,
		withTests:   *withTests,
		withModel:   *withModel,
		withGRPC:    hasGRPC,
		withDB:      *withDB,
		dbDriver:    *dbDriver,
		withSwag:    *withSwag,
		serviceName: *serviceName,
		routePrefix: *routePrefix,
	}
}

func (c config) validate() error {
	if c.idlPath == "" {
		return fmt.Errorf("IDL file path is required (-idl flag)")
	}
	if _, err := os.Stat(c.idlPath); os.IsNotExist(err) {
		return fmt.Errorf("IDL file not found: %s", c.idlPath)
	}
	for _, p := range c.protocols {
		p = strings.TrimSpace(p)
		if p != "http" && p != "grpc" {
			return fmt.Errorf("unsupported protocol: %q (allowed: http, grpc)", p)
		}
	}
	// 驱动校验由 generator.New 负责，这里只做非空检测
	if c.dbDriver == "" {
		return fmt.Errorf("db.driver cannot be empty")
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg := parseFlags()
	if err := cfg.validate(); err != nil {
		log.Fatalf("Invalid configuration: %v\n\nRun with -help for usage.", err)
	}

	// ─── 解析 IDL（完整模式：含结构体/model）───
	log.Printf("Parsing IDL: %s", cfg.idlPath)
	result, err := parser.ParseFull(cfg.idlPath)
	if err != nil {
		log.Fatalf("Failed to parse IDL: %v", err)
	}

	log.Printf("Found %d service(s), %d struct(s) in package %q",
		len(result.Services), len(result.Models), result.PackageName)

	if cfg.serviceName == "" && len(result.Services) > 0 {
		cfg.serviceName = result.Services[0].ServiceName
	}

	// ─── 初始化生成器 ───
	gen, err := generator.New(generator.Options{
		TemplateFS:  &templateFS,
		OutputDir:   cfg.outputDir,
		ImportPath:  cfg.ImportPath,
		ServiceName: cfg.serviceName,
		Protocols:   cfg.protocols,
		WithConfig:  cfg.withConfig,
		WithDocs:    cfg.withDocs,
		WithTests:   cfg.withTests,
		WithModel:   cfg.withModel,
		WithGRPC:    cfg.withGRPC,
		WithDB:      cfg.withDB,
		DBDriver:    cfg.dbDriver,
		WithSwag:    cfg.withSwag,
		IDLSrcPath:  cfg.idlPath,
		RoutePrefix: cfg.routePrefix,
	})
	if err != nil {
		log.Fatalf("Failed to create generator: %v", err)
	}

	// ─── 生成代码 ───
	log.Println("Generating code...")
	if err := gen.GenerateFull(result); err != nil {
		log.Fatalf("Code generation failed: %v", err)
	}

	// ─── 打印摘要 ───
	log.Printf("✅ Code generated in: %s", cfg.outputDir)
	log.Printf("   Services  : %d", len(result.Services))
	for _, svc := range result.Services {
		log.Printf("     - %-30s (%d methods)", svc.ServiceName, len(svc.Methods))
	}
	if cfg.withModel {
		log.Printf("   Models    : %d", len(result.Models))
		for _, m := range result.Models {
			log.Printf("     - %-30s → table: %s", m.Name, m.TableName)
		}
		log.Printf("   DB Driver : %s", cfg.dbDriver)
	}
	if cfg.withGRPC {
		log.Printf("   gRPC      : .proto files written to pb/  ← see protoc hints above")
		log.Printf("   Next step : run the protoc commands printed above to generate pb.go")
	}
	if cfg.withSwag {
		log.Printf("   Swagger   : docs stub written to docs/docs.go")
		log.Printf("   Next step : cd %s && go mod tidy", cfg.outputDir)
		log.Printf("             : go install github.com/swaggo/swag/cmd/swag@latest")
		log.Printf("             : swag init -g cmd/main.go")
		log.Printf("             : go run ./cmd/main.go  # then visit http://localhost:8080/swagger/index.html")
	}
}
