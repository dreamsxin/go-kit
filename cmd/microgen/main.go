package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/dbschema"
	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"

	// 注册数据库驱动（仅在 from-db 模式下实际使用）
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/microsoft/go-mssqldb"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// config 命令行参数配置
type config struct {
	// ── IDL 模式 ──
	idlPath string

	// ── DB 模式 ──
	fromDB   bool     // 是否从数据库生成
	dbDSN    string   // 数据库连接 DSN
	dbName   string   // 数据库名（MySQL/SQLServer 需要）
	dbTables []string // 指定要生成的表（空 = 全部）

	// ── 公共 ──
	outputDir   string
	ImportPath  string
	protocols   []string
	withConfig  bool
	withDocs    bool
	withTests   bool
	withModel   bool
	withGRPC    bool
	withDB      bool
	dbDriver    string
	withSwag    bool
	serviceName string
	routePrefix string
}

func parseFlags() config {
	// ── IDL 模式 ──
	idlPath := flag.String("idl", "", "Path to IDL file (idl mode)")

	// ── DB 模式 ──
	fromDB := flag.Bool("from-db", false, "Generate from database schema instead of IDL file")
	dsn := flag.String("dsn", "", "Database DSN (required for -from-db)")
	dbName := flag.String("dbname", "", "Database name (required for MySQL/SQLServer in -from-db mode)")
	tables := flag.String("tables", "", "Comma-separated table names to generate (empty = all tables)")

	// ── 公共 ──
	outputDir := flag.String("out", ".", "Output directory")
	importPath := flag.String("import", "", "Go module import path for the generated project")
	protocols := flag.String("protocols", "http", "Supported protocols, comma-separated: http,grpc")
	withConfig := flag.Bool("config", true, "Generate config/config.yaml")
	withDocs := flag.Bool("docs", true, "Generate README.md")
	withTests := flag.Bool("tests", false, "Generate unit test files")
	withModel := flag.Bool("model", true, "Generate gorm model & repository layer")
	withDB := flag.Bool("db", true, "Include database initialization in main.go")
	driver := flag.String("driver", "mysql", "Database driver: sqlite, mysql, postgres, sqlserver, clickhouse")
	withSwag := flag.Bool("swag", false, "Generate swaggo annotations + docs stub + /swagger/ UI route")
	serviceName := flag.String("service", "", "Service name (default: derived from -dbname or first IDL interface)")
	routePrefix := flag.String("prefix", "", "HTTP route prefix (e.g. /api/v1)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "microgen - Go microservice code generator\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  # 从 IDL 文件生成\n")
		fmt.Fprintf(os.Stderr, "  microgen -idl <idl_file> -out <dir> -import <path> [options]\n\n")
		fmt.Fprintf(os.Stderr, "  # 从数据库生成\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver <driver> -dsn <dsn> [-dbname <db>] -out <dir> -import <path> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # 从 IDL 生成\n")
		fmt.Fprintf(os.Stderr, "  microgen -idl ./idl.go -out ./gen -import github.com/example/app\n\n")
		fmt.Fprintf(os.Stderr, "  # 从 MySQL 生成（所有表）\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver mysql -dsn \"root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True\" -dbname mydb -out ./gen -import github.com/example/app -service MyApp\n\n")
		fmt.Fprintf(os.Stderr, "  # 从 MySQL 生成（指定表）\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver mysql -dsn \"root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True\" -dbname mydb -tables \"users,orders\" -out ./gen -import github.com/example/app\n\n")
		fmt.Fprintf(os.Stderr, "  # 从 PostgreSQL 生成\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver postgres -dsn \"host=127.0.0.1 user=postgres password=pass dbname=mydb sslmode=disable\" -out ./gen -import github.com/example/app\n\n")
		fmt.Fprintf(os.Stderr, "  # 从 SQLite 生成\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver sqlite -dsn \"app.db\" -out ./gen -import github.com/example/app -service MyApp\n\n")
	}

	flag.Parse()

	protos := strings.Split(*protocols, ",")
	hasGRPC := false
	for _, p := range protos {
		if strings.TrimSpace(p) == "grpc" {
			hasGRPC = true
		}
	}

	var tableList []string
	if *tables != "" {
		for _, t := range strings.Split(*tables, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tableList = append(tableList, t)
			}
		}
	}

	return config{
		idlPath:     *idlPath,
		fromDB:      *fromDB,
		dbDSN:       *dsn,
		dbName:      *dbName,
		dbTables:    tableList,
		outputDir:   *outputDir,
		ImportPath:  *importPath,
		protocols:   protos,
		withConfig:  *withConfig,
		withDocs:    *withDocs,
		withTests:   *withTests,
		withModel:   *withModel,
		withGRPC:    hasGRPC,
		withDB:      *withDB,
		dbDriver:    *driver,
		withSwag:    *withSwag,
		serviceName: *serviceName,
		routePrefix: *routePrefix,
	}
}

func (c config) validate() error {
	if !c.fromDB && c.idlPath == "" {
		return fmt.Errorf("either -idl or -from-db is required")
	}
	if c.fromDB {
		if c.dbDSN == "" {
			return fmt.Errorf("-dsn is required when using -from-db")
		}
		d := strings.ToLower(c.dbDriver)
		if d == "mysql" || d == "sqlserver" || d == "mssql" {
			if c.dbName == "" {
				return fmt.Errorf("-dbname is required for driver %q", c.dbDriver)
			}
		}
	} else {
		if _, err := os.Stat(c.idlPath); os.IsNotExist(err) {
			return fmt.Errorf("IDL file not found: %s", c.idlPath)
		}
	}
	for _, p := range c.protocols {
		p = strings.TrimSpace(p)
		if p != "http" && p != "grpc" {
			return fmt.Errorf("unsupported protocol: %q (allowed: http, grpc)", p)
		}
	}
	if c.dbDriver == "" {
		return fmt.Errorf("driver cannot be empty")
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg := parseFlags()
	if err := cfg.validate(); err != nil {
		log.Fatalf("Invalid configuration: %v\n\nRun with -help for usage.", err)
	}

	var (
		result  *parser.ParseResult
		idlPath string
	)

	if cfg.fromDB {
		result, idlPath = runFromDB(cfg)
	} else {
		result = runFromIDL(cfg)
		idlPath = cfg.idlPath
	}

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
		IDLSrcPath:  idlPath,
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
	}
	if cfg.withSwag {
		log.Printf("   Swagger   : docs stub written to docs/docs.go")
		log.Printf("   Next step : cd %s && go mod tidy", cfg.outputDir)
		log.Printf("             : go install github.com/swaggo/swag/cmd/swag@latest")
		log.Printf("             : swag init -g cmd/main.go")
	}
}

// ─────────────────────────── IDL 模式 ───────────────────────────

func runFromIDL(cfg config) *parser.ParseResult {
	log.Printf("Parsing IDL: %s", cfg.idlPath)
	result, err := parser.ParseFull(cfg.idlPath)
	if err != nil {
		log.Fatalf("Failed to parse IDL: %v", err)
	}
	log.Printf("Found %d service(s), %d struct(s) in package %q",
		len(result.Services), len(result.Models), result.PackageName)
	return result
}

// ─────────────────────────── DB 模式 ───────────────────────────

func runFromDB(cfg config) (*parser.ParseResult, string) {
	driver := strings.ToLower(cfg.dbDriver)
	sqlDriver := gormDriverToSQL(driver)

	log.Printf("Connecting to database [driver=%s] ...", driver)
	db, err := sql.Open(sqlDriver, cfg.dbDSN)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v\nDSN: %s", err, cfg.dbDSN)
	}
	log.Printf("Database connected.")

	// 内省表结构
	intro, err := dbschema.NewIntrospector(driver)
	if err != nil {
		log.Fatalf("Introspector error: %v", err)
	}

	schemas, err := intro.Tables(db, cfg.dbName, cfg.dbTables)
	if err != nil {
		log.Fatalf("Failed to introspect tables: %v", err)
	}
	if len(schemas) == 0 {
		log.Fatalf("No tables found in database %q (tables filter: %v)", cfg.dbName, cfg.dbTables)
	}
	log.Printf("Found %d table(s): %s", len(schemas), tableNames(schemas))

	// 推导服务名
	svcName := cfg.serviceName
	if svcName == "" {
		if cfg.dbName != "" {
			svcName = dbschema.SnakeToCamel(cfg.dbName) + "Service"
		} else {
			svcName = "AppService"
		}
	}

	// 推导包名（取 import path 最后一段）
	pkgName := lastPathSegment(cfg.ImportPath)
	if pkgName == "" {
		pkgName = strings.ToLower(svcName)
	}

	// 生成 idl.go 到输出目录
	log.Printf("Writing idl.go to %s ...", cfg.outputDir)
	idlPath, err := dbschema.WriteIDL(schemas, pkgName, cfg.outputDir)
	if err != nil {
		log.Fatalf("Failed to write idl.go: %v", err)
	}
	log.Printf("idl.go written: %s", idlPath)

	// 直接从 schema 构建 ParseResult（不重新解析 idl.go）
	result := dbschema.ToParseResult(schemas, svcName, pkgName)
	log.Printf("Generated %d service(s) with %d model(s)", len(result.Services), len(result.Models))
	return result, idlPath
}

// gormDriverToSQL 将 gorm 驱动名映射到 database/sql 注册的驱动名
func gormDriverToSQL(driver string) string {
	switch driver {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite3"
	case "sqlserver", "mssql":
		return "sqlserver"
	default:
		return driver
	}
}

func tableNames(schemas []*dbschema.TableSchema) string {
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		names = append(names, s.TableName)
	}
	return strings.Join(names, ", ")
}

func lastPathSegment(importPath string) string {
	parts := strings.Split(strings.TrimRight(importPath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	seg := parts[len(parts)-1]
	seg = strings.NewReplacer("-", "_", ".", "_").Replace(seg)
	return strings.ToLower(seg)
}
