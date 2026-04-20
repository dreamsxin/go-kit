package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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
	checkOnly   bool
	appendSvc   string
	appendModel string
	appendMW    []string
}

func parseFlags() config {
	return parseConfig(flag.CommandLine, os.Args[1:])
}

func newExtendFlagSet(output io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("extend", flag.ExitOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage of microgen extend:")
		fmt.Fprintln(fs.Output(), "  microgen extend -idl <full-combined.go> -out <project> -append-service <Name>")
		fmt.Fprintln(fs.Output(), "  microgen extend -idl <full-combined.go> -out <project> -append-model <Name>")
		fmt.Fprintln(fs.Output(), "  microgen extend -idl <full-combined.go> -out <project> -append-middleware <Name[,Name...]>")
		fmt.Fprintln(fs.Output(), "  microgen extend -check -out <project>")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Current extend mode supports full combined Go IDL input only; .proto input is not supported.")
		fmt.Fprintln(fs.Output())
		fs.PrintDefaults()
	}
	return fs
}

func parseConfig(fs *flag.FlagSet, args []string) config {
	idlPath := fs.String("idl", "", "Path to IDL file (.go or .proto)")
	fromDB := fs.Bool("from-db", false, "Generate from database schema")
	dsn := fs.String("dsn", "", "Database DSN")
	dbName := fs.String("dbname", "", "Database name")
	tables := fs.String("tables", "", "Comma-separated table names")
	addTables := fs.String("add-tables", "", "Comma-separated table names to append")

	outputDir := fs.String("out", ".", "Output directory")
	importPath := fs.String("import", "", "Go module import path")
	protocols := fs.String("protocols", "http", "Supported protocols: http,grpc")
	withConfig := fs.Bool("config", true, "Generate config")
	withDocs := fs.Bool("docs", true, "Generate docs")
	withTests := fs.Bool("tests", false, "Generate tests")
	withModel := fs.Bool("model", true, "Generate model")
	withDB := fs.Bool("db", true, "Include DB init in main")
	driver := fs.String("driver", "mysql", "Database driver")
	withSwag := fs.Bool("swag", false, "Generate swagger")
	withSkill := fs.Bool("skill", true, "Generate AI skill support")
	serviceName := fs.String("service", "", "Service name")
	routePrefix := fs.String("prefix", "", "HTTP route prefix")
	checkOnly := fs.Bool("check", false, "Scan an existing project and print extend compatibility without changing files")
	appendSvc := fs.String("append-service", "", "Append exactly one new service in extend mode")
	appendModel := fs.String("append-model", "", "Append exactly one new model in extend mode")
	appendMiddleware := fs.String("append-middleware", "", "Append generated middleware names in extend mode (supported: tracing,error-handling,metrics)")

	_ = fs.Parse(args)

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
		checkOnly:   *checkOnly,
		appendSvc:   *appendSvc,
		appendModel: *appendModel,
		appendMW:    splitComma(*appendMiddleware),
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
	if len(os.Args) > 1 && os.Args[1] == "extend" {
		runExtend()
		return
	}

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
	log.Printf("Done! Output: %s", cfg.outputDir)
}

func runExtend() {
	fs := newExtendFlagSet(os.Stderr)
	cfg := parseConfig(fs, os.Args[2:])
	if err := cfg.validateExtend(); err != nil {
		log.Fatal(err)
	}
	if cfg.checkOnly {
		existing, err := generator.ScanExistingProject(cfg.outputDir)
		if err != nil {
			log.Fatalf("extend check failed: %v", err)
		}
		printExtendCheckReport(os.Stdout, existing)
		if code := extendCheckExitCode(existing); code != 0 {
			os.Exit(code)
		}
		return
	}
	project, err := runFromIDL(cfg)
	if err != nil {
		log.Fatal(err)
	}
	extendOpts := generator.ExtendOptions{
		AppendService:    cfg.appendSvc,
		AppendModel:      cfg.appendModel,
		AppendMiddleware: cfg.appendMW,
	}
	if cfg.appendSvc != "" {
		if _, err := generator.ApplyAppendService(&templateFS, cfg.outputDir, project, extendOpts, cfg.idlPath); err != nil {
			log.Fatalf("extend append-service failed: %v", err)
		}
		log.Printf("append-service complete: %s -> %s", cfg.appendSvc, cfg.outputDir)
		return
	}
	if len(cfg.appendMW) > 0 {
		if _, err := generator.ApplyAppendMiddleware(&templateFS, cfg.outputDir, project, extendOpts, cfg.idlPath); err != nil {
			log.Fatalf("extend append-middleware failed: %v", err)
		}
		log.Printf("append-middleware complete: %s -> %s", strings.Join(cfg.appendMW, ","), cfg.outputDir)
		return
	}
	if _, err := generator.ApplyAppendModel(&templateFS, cfg.outputDir, project, extendOpts, cfg.idlPath); err != nil {
		log.Fatalf("extend append-model failed: %v", err)
	}
	log.Printf("append-model complete: %s -> %s", cfg.appendModel, cfg.outputDir)
}

func (c config) validateExtend() error {
	if c.fromDB {
		return fmt.Errorf("extend mode does not support -from-db; use -idl with a full combined Go IDL contract instead")
	}
	if c.checkOnly {
		targetCount := 0
		if c.appendSvc != "" {
			targetCount++
		}
		if c.appendModel != "" {
			targetCount++
		}
		if len(c.appendMW) > 0 {
			targetCount++
		}
		if targetCount > 0 {
			return fmt.Errorf("extend mode -check cannot be combined with -append-service, -append-model, or -append-middleware")
		}
		if c.outputDir == "" {
			return fmt.Errorf("extend mode -check requires -out with an existing generated project path")
		}
		return nil
	}
	if c.idlPath == "" {
		return fmt.Errorf("extend mode requires -idl with a full combined Go IDL contract")
	}
	if strings.HasSuffix(strings.ToLower(c.idlPath), ".proto") {
		return fmt.Errorf("extend mode currently supports Go IDL only; .proto input is not supported for -append-service, -append-model, or -append-middleware")
	}
	targetCount := 0
	if c.appendSvc != "" {
		targetCount++
	}
	if c.appendModel != "" {
		targetCount++
	}
	if len(c.appendMW) > 0 {
		targetCount++
	}
	if targetCount > 1 {
		return fmt.Errorf("extend mode supports only one mutation target at a time; choose either -append-service, -append-model, or -append-middleware")
	}
	if targetCount == 0 {
		return fmt.Errorf("extend mode currently requires -append-service <Name>, -append-model <Name>, or -append-middleware <Name[,Name...]>")
	}
	return nil
}

func printExtendCheckReport(w io.Writer, existing *generator.ExistingProject) {
	if existing == nil {
		fmt.Fprintln(w, "Extend compatibility check failed: no project data")
		return
	}
	overallReady := extendCheckExitCode(existing) == 0
	fmt.Fprintf(w, "Extend compatibility for %s\n", existing.Root)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Summary:")
	fmt.Fprintf(w, "- Module: %s\n", existing.ModulePath)
	fmt.Fprintf(w, "- Overall status: %s\n", readinessLabel(overallReady))
	fmt.Fprintf(w, "- Services: %d\n", len(existing.Services))
	fmt.Fprintf(w, "- Models: %d\n", len(existing.Models))
	if len(existing.Features.GeneratedMiddlewares) > 0 {
		fmt.Fprintf(w, "- Generated middleware: %s\n", strings.Join(existing.Features.GeneratedMiddlewares, ", "))
	} else {
		fmt.Fprintln(w, "- Generated middleware: none")
	}

	printSeamStatus := func(label, path string) {
		if path == "" {
			fmt.Fprintf(w, "- %s: missing\n", label)
			return
		}
		rel, err := filepath.Rel(existing.Root, path)
		if err != nil {
			rel = path
		}
		fmt.Fprintf(w, "- %s: %s\n", label, filepath.ToSlash(rel))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Compatibility Seams:")
	printSeamStatus("Generated services seam", existing.AggregationPoints.GeneratedServices)
	printSeamStatus("Generated routes seam", existing.AggregationPoints.GeneratedRoutes)
	printSeamStatus("Generated runtime seam", existing.AggregationPoints.GeneratedRuntime)

	appendServiceMissing := appendServiceMissingSeams(existing)
	appendModelMissing := appendModelMissingSeams(existing)
	appendMiddlewareMissing := appendMiddlewareMissingSeams(existing)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Append Paths:")
	printAppendPathStatus(w, "append-service", appendServiceMissing)
	printAppendPathStatus(w, "append-model", appendModelMissing)
	printAppendPathStatus(w, "append-middleware", appendMiddlewareMissing)

	if len(existing.Warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Warnings:")
		for _, warning := range existing.Warnings {
			fmt.Fprintf(w, "- %s\n", warning)
		}
	}
}

func readinessLabel(ok bool) string {
	if ok {
		return "ready"
	}
	return "needs compatibility seams"
}

func printAppendPathStatus(w io.Writer, name string, missing []string) {
	if len(missing) == 0 {
		fmt.Fprintf(w, "- %s: %s\n", name, readinessLabel(true))
		return
	}
	fmt.Fprintf(w, "- %s: %s (missing: %s)\n", name, readinessLabel(false), strings.Join(missing, ", "))
}

func appendServiceMissingSeams(existing *generator.ExistingProject) []string {
	var missing []string
	if existing == nil {
		return []string{"project scan data"}
	}
	if existing.AggregationPoints.GeneratedServices == "" {
		missing = append(missing, "cmd/generated_services.go")
	}
	if existing.AggregationPoints.GeneratedRoutes == "" {
		missing = append(missing, "cmd/generated_routes.go")
	}
	return missing
}

func appendModelMissingSeams(existing *generator.ExistingProject) []string {
	if existing == nil {
		return []string{"project scan data"}
	}
	var missing []string
	if !existing.Features.WithModel {
		return []string{"generated model/repository output"}
	}
	if existing.Features.WithDB {
		if existing.AggregationPoints.GeneratedServices == "" {
			missing = append(missing, "cmd/generated_services.go")
		}
		if existing.AggregationPoints.GeneratedRuntime == "" {
			missing = append(missing, "cmd/generated_runtime.go")
		}
	}
	for _, svc := range existing.Services {
		rel := filepath.ToSlash(filepath.Join("service", svc.PackageName, "generated_repos.go"))
		own, ok := existing.Ownership[rel]
		if !ok || own.Tier != generator.OwnershipGeneratorRebuildable {
			missing = append(missing, rel)
		}
	}
	return missing
}

func appendMiddlewareMissingSeams(existing *generator.ExistingProject) []string {
	if existing == nil {
		return []string{"project scan data"}
	}
	var missing []string
	if existing.AggregationPoints.GeneratedRoutes == "" {
		missing = append(missing, "cmd/generated_routes.go")
	}
	for _, svc := range existing.Services {
		rel := filepath.ToSlash(filepath.Join("endpoint", svc.PackageName, "generated_chain.go"))
		own, ok := existing.Ownership[rel]
		if !ok || own.Tier != generator.OwnershipGeneratorRebuildable {
			missing = append(missing, rel)
		}
	}
	return missing
}

func extendCheckExitCode(existing *generator.ExistingProject) int {
	if existing == nil {
		return 2
	}
	if len(appendServiceMissingSeams(existing)) > 0 {
		return 2
	}
	if len(appendModelMissingSeams(existing)) > 0 {
		return 2
	}
	if len(appendMiddlewareMissingSeams(existing)) > 0 {
		return 2
	}
	return 0
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
