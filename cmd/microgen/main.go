package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/dbschema"
	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"

	// Register database drivers (only used in from-db mode)
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
	addTables []string // 追加新表到已有项目（与 dbTables 互斥）

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
	addTables := flag.String("add-tables", "", "Comma-separated table names to append to an existing generated project")

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
		fmt.Fprintf(os.Stderr, "  # From IDL file\n")
		fmt.Fprintf(os.Stderr, "  microgen -idl <idl_file> -out <dir> -import <path> [options]\n\n")
		fmt.Fprintf(os.Stderr, "  # From database (full generation)\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver <driver> -dsn <dsn> [-dbname <db>] -out <dir> -import <path> [options]\n\n")
		fmt.Fprintf(os.Stderr, "  # Append new tables to an existing generated project\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver <driver> -dsn <dsn> [-dbname <db>] -add-tables <t1,t2> -out <dir> -import <path>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Generate from IDL\n")
		fmt.Fprintf(os.Stderr, "  microgen -idl ./idl.go -out ./gen -import github.com/example/app\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate from MySQL (all tables)\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver mysql -dsn \"root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True\" -dbname mydb -out ./gen -import github.com/example/app -service MyApp\n\n")
		fmt.Fprintf(os.Stderr, "  # Append new tables to existing project\n")
		fmt.Fprintf(os.Stderr, "  microgen -from-db -driver mysql -dsn \"root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True\" -dbname mydb -add-tables \"orders,products\" -out ./gen -import github.com/example/app\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate from SQLite\n")
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

	var addTableList []string
	if *addTables != "" {
		for _, t := range strings.Split(*addTables, ",") {
			if t = strings.TrimSpace(t); t != "" {
				addTableList = append(addTableList, t)
			}
		}
	}

	return config{
		idlPath:     *idlPath,
		fromDB:      *fromDB,
		dbDSN:       *dsn,
		dbName:      *dbName,
		dbTables:    tableList,
		addTables:   addTableList,
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
		if len(c.addTables) > 0 && len(c.dbTables) > 0 {
			return fmt.Errorf("-tables and -add-tables are mutually exclusive")
		}
		if len(c.addTables) > 0 {
			// add-tables mode: output dir must already contain idl.go
			idlPath := filepath.Join(c.outputDir, "idl.go")
			if _, err := os.Stat(idlPath); os.IsNotExist(err) {
				return fmt.Errorf("-add-tables requires an existing generated project at %q (idl.go not found)", c.outputDir)
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

// ─────────────────────────── DB mode ───────────────────────────

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

	intro, err := dbschema.NewIntrospector(driver)
	if err != nil {
		log.Fatalf("Introspector error: %v", err)
	}

	// ── Determine which tables to introspect ──
	tableFilter := cfg.dbTables
	if len(cfg.addTables) > 0 {
		tableFilter = cfg.addTables
	}

	schemas, err := intro.Tables(db, cfg.dbName, tableFilter)
	if err != nil {
		log.Fatalf("Failed to introspect tables: %v", err)
	}
	if len(schemas) == 0 {
		log.Fatalf("No tables found (filter: %v)", tableFilter)
	}
	log.Printf("Introspected %d table(s): %s", len(schemas), tableNames(schemas))

	// ── add-tables: merge with existing idl.go ──
	if len(cfg.addTables) > 0 {
		schemas = mergeWithExisting(cfg.outputDir, schemas)
	}

	// ── Derive service name and package name ──
	svcName := cfg.serviceName
	if svcName == "" {
		// Try to read service name from existing idl.go first
		if existingSvc := readExistingServiceName(cfg.outputDir); existingSvc != "" {
			svcName = existingSvc
		} else if cfg.dbName != "" {
			svcName = dbschema.SnakeToCamel(cfg.dbName) + "Service"
		} else {
			svcName = "AppService"
		}
	}

	pkgName := lastPathSegment(cfg.ImportPath)
	if pkgName == "" {
		pkgName = strings.ToLower(svcName)
	}

	// When the user explicitly provided -service, use its lowercase as the IDL
	// package name so that WriteIDL derives the correct service interface name.
	idlPkgName := pkgName
	if cfg.serviceName != "" {
		idlPkgName = strings.ToLower(strings.TrimSuffix(svcName, "Service"))
		if idlPkgName == "" {
			idlPkgName = pkgName
		}
	}

	// ── Write merged idl.go ──
	log.Printf("Writing idl.go to %s ...", cfg.outputDir)
	idlPath, err := dbschema.WriteIDL(schemas, idlPkgName, cfg.outputDir)
	if err != nil {
		log.Fatalf("Failed to write idl.go: %v", err)
	}
	log.Printf("idl.go written: %s", idlPath)

	result := dbschema.ToParseResult(schemas, svcName, pkgName)
	log.Printf("Total: %d service(s), %d model(s)", len(result.Services), len(result.Models))
	return result, idlPath
}

// mergeWithExisting reads the existing idl.go in outDir, extracts already-generated
// table names, and prepends them to newSchemas so the full set is regenerated.
func mergeWithExisting(outDir string, newSchemas []*dbschema.TableSchema) []*dbschema.TableSchema {
	idlPath := filepath.Join(outDir, "idl.go")
	existing, err := parser.ParseFull(idlPath)
	if err != nil {
		log.Printf("[warn] could not parse existing idl.go (%v); treating as empty", err)
		return newSchemas
	}

	// Build set of already-present table names (from existing models)
	existingTables := make(map[string]bool, len(existing.Models))
	for _, m := range existing.Models {
		if m.HasGormTags {
			existingTables[m.TableName] = true
		}
	}

	// Convert existing GORM models back to TableSchema stubs so WriteIDL can re-emit them.
	// Only include models with gorm tags (actual DB tables), not DTOs.
	var merged []*dbschema.TableSchema
	for _, m := range existing.Models {
		if m.HasGormTags {
			merged = append(merged, dbschema.ModelToSchema(m))
		}
	}

	// Append only genuinely new tables
	added := 0
	for _, s := range newSchemas {
		if existingTables[s.TableName] {
			log.Printf("[skip] table %q already exists in idl.go", s.TableName)
			continue
		}
		merged = append(merged, s)
		added++
	}
	log.Printf("Merged: %d existing + %d new table(s)", len(existing.Models), added)
	return merged
}

// readExistingServiceName reads the service name from an existing idl.go.
func readExistingServiceName(outDir string) string {
	idlPath := filepath.Join(outDir, "idl.go")
	existing, err := parser.ParseFull(idlPath)
	if err != nil || len(existing.Services) == 0 {
		return ""
	}
	return existing.Services[0].ServiceName
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
