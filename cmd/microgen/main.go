package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/dbschema"
	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

type config struct {
	idlPath   string
	fromDB    bool
	dbDSN     string
	dbName    string
	dbTables  []string
	addTables []string

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
	withSkill   bool
	serviceName string
	routePrefix string
}

func parseFlags() config {
	idlPath := flag.String("idl", "", "Path to IDL file (.go or .proto)")
	fromDB := flag.Bool("from-db", false, "Generate from database schema")
	dsn := flag.String("dsn", "", "Database DSN")
	dbName := flag.String("dbname", "", "Database name")
	tables := flag.String("tables", "", "Comma-separated table names")
	addTables := flag.String("add-tables", "", "Comma-separated table names to append")

	outputDir := flag.String("out", ".", "Output directory")
	importPath := flag.String("import", "", "Go module import path")
	protocols := flag.String("protocols", "http", "Supported protocols: http,grpc")
	withConfig := flag.Bool("config", true, "Generate config")
	withDocs := flag.Bool("docs", true, "Generate docs")
	withTests := flag.Bool("tests", false, "Generate tests")
	withModel := flag.Bool("model", true, "Generate model")
	withDB := flag.Bool("db", true, "Include DB init in main")
	driver := flag.String("driver", "mysql", "Database driver")
	withSwag := flag.Bool("swag", false, "Generate swagger")
	withSkill := flag.Bool("skill", true, "Generate AI skill support")
	serviceName := flag.String("service", "", "Service name")
	routePrefix := flag.String("prefix", "", "HTTP route prefix")

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
		fromDB:      *fromDB,
		dbDSN:       *dsn,
		dbName:      *dbName,
		dbTables:    splitComma(*tables),
		addTables:   splitComma(*addTables),
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
		withSkill:   *withSkill,
		serviceName: *serviceName,
		routePrefix: *routePrefix,
	}
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	return result
}

func (c config) validate() error {
	if !c.fromDB && c.idlPath == "" {
		return fmt.Errorf("either -idl or -from-db is required")
	}
	return nil
}

func main() {
	cfg := parseFlags()
	if err := cfg.validate(); err != nil {
		log.Fatal(err)
	}

	var (
		project *ir.Project
		idlPath string
		err     error
	)

	if cfg.fromDB {
		project, idlPath, err = runFromDB(cfg)
	} else {
		project, err = runFromIDL(cfg)
		idlPath = cfg.idlPath
	}
	if err != nil {
		log.Fatal(err)
	}

	if cfg.serviceName == "" && len(project.Services) > 0 {
		cfg.serviceName = project.Services[0].Name
	}

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
		WithSkill:   cfg.withSkill,
		IDLSrcPath:  idlPath,
		RoutePrefix: cfg.routePrefix,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Generating code...")
	if err := gen.GenerateIR(project); err != nil {
		log.Fatal(err)
	}
	log.Printf("✅ Done! Output: %s", cfg.outputDir)
}

func runFromIDL(cfg config) (*ir.Project, error) {
	var (
		result *parser.ParseResult
		err    error
	)
	if strings.HasSuffix(cfg.idlPath, ".proto") {
		result, err = parser.ParseProto(cfg.idlPath)
	} else {
		result, err = parser.ParseFull(cfg.idlPath)
	}
	if err != nil {
		return nil, err
	}
	return ir.FromParseResult(result), nil
}

func runFromDB(cfg config) (*ir.Project, string, error) {
	// Minimal implementation for now to keep the example clean
	// In a real scenario, this would call dbschema logic
	sqlDriver := cfg.dbDriver
	switch strings.ToLower(sqlDriver) {
	case "sqlite":
		sqlDriver = "sqlite3"
	}
	db, err := sql.Open(sqlDriver, cfg.dbDSN)
	if err != nil {
		return nil, "", err
	}
	defer db.Close()

	intro, err := dbschema.NewIntrospector(cfg.dbDriver)
	if err != nil {
		return nil, "", err
	}
	schemas, err := intro.Tables(db, cfg.dbName, cfg.dbTables)
	if err != nil {
		return nil, "", err
	}
	pkgName := "gen"
	serviceName := cfg.serviceName
	if serviceName == "" {
		serviceName = "GenService"
	}
	idlPath, err := dbschema.WriteIDL(schemas, pkgName, cfg.outputDir)
	if err != nil {
		return nil, "", err
	}
	project := ir.FromTableSchemas(schemas, serviceName, pkgName)
	return project, idlPath, nil
}
