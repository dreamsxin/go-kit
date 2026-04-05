package generator

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

var supportedDrivers = map[string]struct {
	Driver     string
	ImportPkg  string
	OpenCall   string
	DefaultDSN string
	ConfigDSN  string
}{
	"mysql": {
		Driver:     "mysql",
		ImportPkg:  "gorm.io/driver/mysql",
		OpenCall:   "mysql.Open(*dsn)",
		DefaultDSN: "root:password@tcp(127.0.0.1:3306)/{svcname}?charset=utf8mb4&parseTime=True&loc=Local",
		ConfigDSN:  "root:password@tcp(127.0.0.1:3306)/{svcname}?charset=utf8mb4&parseTime=True&loc=Local",
	},
	"postgres": {
		Driver:     "postgres",
		ImportPkg:  "gorm.io/driver/postgres",
		OpenCall:   "postgres.Open(*dsn)",
		DefaultDSN: "host=127.0.0.1 user=postgres password=password dbname={svcname} port=5432 sslmode=disable",
		ConfigDSN:  "host=127.0.0.1 user=postgres password=password dbname={svcname} port=5432 sslmode=disable",
	},
	"sqlserver": {
		Driver:     "sqlserver",
		ImportPkg:  "gorm.io/driver/sqlserver",
		OpenCall:   "sqlserver.Open(*dsn)",
		DefaultDSN: "sqlserver://sa:password@127.0.0.1:1433?database={svcname}",
		ConfigDSN:  "sqlserver://sa:password@127.0.0.1:1433?database={svcname}",
	},
	"clickhouse": {
		Driver:     "clickhouse",
		ImportPkg:  "gorm.io/driver/clickhouse",
		OpenCall:   "clickhouse.Open(*dsn)",
		DefaultDSN: "tcp://127.0.0.1:9000?database={svcname}&username=default&password=&read_timeout=10&write_timeout=20",
		ConfigDSN:  "tcp://127.0.0.1:9000?database={svcname}&username=default&password=&read_timeout=10&write_timeout=20",
	},
	"sqlite": {
		Driver:     "sqlite",
		ImportPkg:  "gorm.io/driver/sqlite",
		OpenCall:   "sqlite.Open(*dsn)",
		DefaultDSN: "app.db",
		ConfigDSN:  "app.db",
	},
}

// Options 生成器配置选项
type Options struct {
	TemplateFS  fs.FS    // 模板文件系统
	OutputDir   string   // 输出目录
	ImportPath  string   // 项目导入路径
	ServiceName string   // 服务名称
	Protocols   []string // 支持的协议 (http, grpc)
	WithConfig  bool     // 是否生成配置文件
	WithDocs    bool     // 是否生成文档（README）
	WithTests   bool     // 是否生成测试文件
	WithModel   bool     // 是否生成 gorm model + repository
	WithGRPC    bool     // 是否生成 gRPC 传输层
	WithDB      bool     // main 是否包含数据库初始化代码
	DBDriver    string   // 数据库驱动
	WithSwag    bool     // 是否生成 swaggo 注释
	WithSkill   bool     // 是否生成 AI Skill (MCP server) 支持
	IDLSrcPath  string   // IDL 源文件路径
	RoutePrefix string   // HTTP 路由前缀
}

// Generator 核心生成器
type Generator struct {
	config    Options
	templates *template.Template
	outputDir string // 绝对路径
}

// New 创建生成器实例
func New(opt Options) (*Generator, error) {
	if opt.OutputDir == "" {
		opt.OutputDir = "."
	}
	absOut, err := filepath.Abs(opt.OutputDir)
	if err != nil {
		return nil, err
	}

	if opt.DBDriver != "" {
		if _, ok := supportedDrivers[opt.DBDriver]; !ok && opt.DBDriver != "sqlite" {
			return nil, fmt.Errorf("unsupported db driver: %s", opt.DBDriver)
		}
	}

	for _, p := range opt.Protocols {
		if strings.ToLower(p) == "grpc" {
			opt.WithGRPC = true
		}
	}

	tmpl := template.New("microgen").Funcs(template.FuncMap{
		"lower": func(s string) string { return strings.ToLower(s) },
		"upper": func(s string) string { return strings.ToUpper(s) },
		"title": func(s string) string { return strings.Title(s) }, //nolint:staticcheck
		"snake":     toSnakeCase,
		"trimStar":  func(s string) string { return strings.TrimPrefix(s, "*") },
		"hasPrefix": strings.HasPrefix,
		"marshal": func(v any) string {
			a, _ := json.Marshal(v)
			return string(a)
		},
		"escape": func(s string) string {
			return strings.ReplaceAll(s, "\"", "\\\"")
		},
	})

	if opt.TemplateFS != nil {
		tmpl, err = tmpl.ParseFS(opt.TemplateFS, "templates/*.tmpl")
		if err != nil {
			return nil, err
		}
	}

	return &Generator{
		config:    opt,
		templates: tmpl,
		outputDir: absOut,
	}, nil
}

// SvcRoute helper for main.tmpl
type SvcRoute struct {
	Service    *parser.Service
	FullPrefix string
}

// GenerateFull 执行完整生成流程
func (g *Generator) GenerateFull(result *parser.ParseResult) error {
	if err := g.createDirStructure(result); err != nil {
		return fmt.Errorf("create dir structure failed: %w", err)
	}

	if g.config.ImportPath != "" {
		if err := g.generateGoModFile(); err != nil {
			return fmt.Errorf("generate go.mod failed: %w", err)
		}
	}

	if g.config.IDLSrcPath != "" && !strings.HasSuffix(g.config.IDLSrcPath, ".proto") {
		if err := g.copyIDLFile(g.config.IDLSrcPath); err != nil {
			return fmt.Errorf("copy idl file failed: %w", err)
		}
	}

	if g.config.WithModel {
		var hasModels bool
		for _, model := range result.Models {
			if !model.HasGormTags {
				continue
			}
			hasModels = true
			if err := g.generateModelFile(model); err != nil {
				return fmt.Errorf("generate model[%s] failed: %w", model.Name, err)
			}
			if err := g.generateRepositoryFile(model); err != nil {
				return fmt.Errorf("generate repository[%s] failed: %w", model.Name, err)
			}
		}
		if hasModels {
			if err := g.generateRepositoryBaseFile(); err != nil {
				return fmt.Errorf("generate repository base failed: %w", err)
			}
		}
	}

	for _, service := range result.Services {
		if err := g.generateServiceFileFull(service, result.Models, result.Source); err != nil {
			return fmt.Errorf("generate service[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateEndpointsFile(service, result.Source); err != nil {
			return fmt.Errorf("generate endpoints[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateHTTPTransportFile(service, result.Source); err != nil {
			return fmt.Errorf("generate http transport[%s] failed: %w", service.ServiceName, err)
		}
		if g.config.WithGRPC {
			if err := g.generateGRPCTransportFile(service, result.Source); err != nil {
				return fmt.Errorf("generate grpc transport[%s] failed: %w", service.ServiceName, err)
			}
			if err := g.generateProtoFile(service); err != nil {
				return fmt.Errorf("generate proto[%s] failed: %w", service.ServiceName, err)
			}
		}
		if g.config.WithTests {
			if err := g.generateTestFile(service, result.Source); err != nil {
				return fmt.Errorf("generate test[%s] failed: %w", service.ServiceName, err)
			}
		}
		if err := g.generateClientDemo(service, result.Source); err != nil {
			return fmt.Errorf("generate client[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateSDKFile(service, result.Source); err != nil {
			return fmt.Errorf("generate sdk[%s] failed: %w", service.ServiceName, err)
		}
	}

	if err := g.generateMainFileFull(result.Services, result.Models); err != nil {
		return fmt.Errorf("generate main failed: %w", err)
	}

	if g.config.WithConfig {
		if err := g.generateConfigFile(result.Services); err != nil {
			return fmt.Errorf("generate config.yaml failed: %w", err)
		}
		if err := g.generateConfigCodeFile(result.Services); err != nil {
			return fmt.Errorf("generate config.go failed: %w", err)
		}
	}

	if g.config.WithDocs {
		if err := g.generateReadme(result.Services); err != nil {
			return fmt.Errorf("generate readme failed: %w", err)
		}
	}

	if g.config.WithSwag {
		if err := g.generateDocsStub(result.Services); err != nil {
			return fmt.Errorf("generate docs stub failed: %w", err)
		}
	}

	if g.config.WithSkill {
		if err := g.generateSkillFile(result); err != nil {
			return fmt.Errorf("generate skill file failed: %w", err)
		}
	}

	return nil
}

func (g *Generator) createDirStructure(result *parser.ParseResult) error {
	dirs := []string{
		filepath.Join(g.outputDir, "cmd"),
	}

	for _, svc := range result.Services {
		pkg := strings.ToLower(svc.ServiceName)
		dirs = append(dirs,
			filepath.Join(g.outputDir, "service", pkg),
			filepath.Join(g.outputDir, "endpoint", pkg),
			filepath.Join(g.outputDir, "transport", pkg),
			filepath.Join(g.outputDir, "client", pkg),
			filepath.Join(g.outputDir, "sdk", pkg+"sdk"),
		)
	}

	if g.config.WithConfig {
		dirs = append(dirs, filepath.Join(g.outputDir, "config"))
	}

	if g.config.WithModel {
		dirs = append(dirs,
			filepath.Join(g.outputDir, "model"),
			filepath.Join(g.outputDir, "repository"),
		)
	}

	if g.config.WithSkill {
		dirs = append(dirs, filepath.Join(g.outputDir, "skill"))
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) generateServiceFileFull(service *parser.Service, models []*parser.Model, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"Models":     models,
		"WithModel":  g.config.WithModel,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	pkg := strings.ToLower(service.ServiceName)
	return g.executeTemplate("service.tmpl", filepath.Join(g.outputDir, "service", pkg, "service.go"), data)
}

func (g *Generator) generateEndpointsFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	pkg := strings.ToLower(service.ServiceName)
	return g.executeTemplate("endpoints.tmpl", filepath.Join(g.outputDir, "endpoint", pkg, "endpoints.go"), data)
}

func (g *Generator) generateHTTPTransportFile(service *parser.Service, source parser.SourceType) error {
	prefix := g.config.RoutePrefix
	if prefix != "" {
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		prefix += strings.ToLower(service.ServiceName)
	}

	data := map[string]any{
		"Service":     service,
		"ImportPath":  g.config.ImportPath,
		"RoutePrefix": prefix,
		"Source":      source,
	}
	pkg := strings.ToLower(service.ServiceName)
	return g.executeTemplate("transport.tmpl", filepath.Join(g.outputDir, "transport", pkg, "transport_http.go"), data)
}

func (g *Generator) generateGRPCTransportFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	pkg := strings.ToLower(service.ServiceName)
	return g.executeTemplate("transport_grpc.tmpl", filepath.Join(g.outputDir, "transport", pkg, "transport_grpc.go"), data)
}

func (g *Generator) generateProtoFile(service *parser.Service) error {
	data := map[string]any{
		"Service": service,
	}
	pkg := strings.ToLower(service.ServiceName)
	pbDir := filepath.Join(g.outputDir, "pb", pkg)
	os.MkdirAll(pbDir, 0755)
	return g.executeTemplate("proto.tmpl", filepath.Join(pbDir, pkg+".proto"), data)
}

func (g *Generator) generateTestFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"Source":     source,
	}
	testDir := filepath.Join(g.outputDir, "test")
	os.MkdirAll(testDir, 0755)
	pkg := strings.ToLower(service.ServiceName)
	return g.executeTemplate("service_test.tmpl", filepath.Join(testDir, pkg+"_test.go"), data)
}

func (g *Generator) generateClientDemo(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"WithGRPC":   g.config.WithGRPC,
		"Source":     source,
	}
	pkg := strings.ToLower(service.ServiceName)
	return g.executeTemplate("client.tmpl", filepath.Join(g.outputDir, "client", pkg, "demo.go"), data)
}

func (g *Generator) generateSDKFile(service *parser.Service, source parser.SourceType) error {
	data := map[string]any{
		"Service":     service,
		"ImportPath":  g.config.ImportPath,
		"WithGRPC":    g.config.WithGRPC,
		"Source":      source,
		"RoutePrefix": g.config.RoutePrefix,
	}
	sdkDir := filepath.Join(g.outputDir, "sdk", strings.ToLower(service.ServiceName)+"sdk")
	os.MkdirAll(sdkDir, 0755)
	return g.executeTemplate("sdk.tmpl", filepath.Join(sdkDir, "client.go"), data)
}

func (g *Generator) generateMainFileFull(services []*parser.Service, models []*parser.Model) error {
	var svcRoutes []SvcRoute
	for _, svc := range services {
		prefix := g.config.RoutePrefix
		if prefix != "" {
			if !strings.HasPrefix(prefix, "/") {
				prefix = "/" + prefix
			}
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			prefix += strings.ToLower(svc.ServiceName)
		}
		svcRoutes = append(svcRoutes, SvcRoute{
			Service:    svc,
			FullPrefix: prefix,
		})
	}

	dbMeta, _ := supportedDrivers[g.config.DBDriver]

	data := map[string]any{
		"Services":     services,
		"Models":       models,
		"GormModels":   models,
		"SvcRoutes":    svcRoutes,
		"ImportPath":   g.config.ImportPath,
		"WithDB":       g.config.WithDB,
		"DBDriver":     g.config.DBDriver,
		"DBImportPkg":  dbMeta.ImportPkg,
		"DBOpenCall":   dbMeta.OpenCall,
		"DBDefaultDSN": dbMeta.DefaultDSN,
		"WithConfig":   g.config.WithConfig,
		"WithGRPC":     g.config.WithGRPC,
		"WithSwag":     g.config.WithSwag,
		"WithSkill":    g.config.WithSkill,
	}
	return g.executeTemplate("main.tmpl", filepath.Join(g.outputDir, "cmd/main.go"), data)
}

func (g *Generator) generateConfigFile(services []*parser.Service) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Services":     services,
		"DBDriver":     g.config.DBDriver,
		"DBDefaultDSN": dbMeta.DefaultDSN,
		"WithGRPC":     g.config.WithGRPC,
		"WithSwag":     g.config.WithSwag,
		"WithDB":       g.config.WithDB,
	}
	return g.executeTemplate("config.tmpl", filepath.Join(g.outputDir, "config/config.yaml"), data)
}

func (g *Generator) generateConfigCodeFile(services []*parser.Service) error {
	dbMeta, _ := supportedDrivers[g.config.DBDriver]
	data := map[string]any{
		"Services":     services,
		"WithDB":       g.config.WithDB,
		"WithGRPC":     g.config.WithGRPC,
		"WithSwag":     g.config.WithSwag,
		"DBDriver":     g.config.DBDriver,
		"DBDefaultDSN": dbMeta.DefaultDSN,
	}
	return g.executeTemplate("config_code.tmpl", filepath.Join(g.outputDir, "config/config.go"), data)
}

func (g *Generator) generateReadme(services []*parser.Service) error {
	data := map[string]any{
		"Services": services,
	}
	return g.executeTemplate("readme.tmpl", filepath.Join(g.outputDir, "README.md"), data)
}

func (g *Generator) generateDocsStub(services []*parser.Service) error {
	docsDir := filepath.Join(g.outputDir, "docs")
	os.MkdirAll(docsDir, 0755)
	path := filepath.Join(docsDir, "docs.go")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	data := map[string]any{
		"Services": services,
	}
	return g.executeTemplate("docs.tmpl", path, data)
}

func (g *Generator) generateSkillFile(result *parser.ParseResult) error {
	data := map[string]any{
		"Services":   result.Services,
		"Models":     result.Models,
		"ImportPath": g.config.ImportPath,
		"Source":     result.Source,
	}
	return g.executeTemplate("skill.tmpl", filepath.Join(g.outputDir, "skill/skill.go"), data)
}

func (g *Generator) generateGoModFile() error {
	path := filepath.Join(g.outputDir, "go.mod")
	if data, err := os.ReadFile(path); err == nil {
		content := string(data)
		if strings.Contains(content, "module "+g.config.ImportPath) {
			return nil
		}
		// 简单的 module 名替换
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "module ") {
				lines[i] = "module " + g.config.ImportPath
				break
			}
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	}

	data := map[string]any{
		"ImportPath": g.config.ImportPath,
		// Hack for local testing in the same repo
		"RootRelPath": "../../",
	}
	// If we are in tools/testdata/gen_idl_integration, we need ../../../
	if strings.Contains(g.outputDir, "testdata") {
		data["RootRelPath"] = "../../../"
	} else if strings.Contains(g.outputDir, "examples") {
		data["RootRelPath"] = "../../"
	}
	return g.executeTemplate("go_mod.tmpl", filepath.Join(g.outputDir, "go.mod"), data)
}

func (g *Generator) generateModelFile(model *parser.Model) error {
	data := map[string]any{
		"Models":     []*parser.Model{model},
		"ImportPath": g.config.ImportPath,
	}
	if err := g.executeTemplate("model.tmpl", filepath.Join(g.outputDir, "model", "model.go"), data); err != nil {
		return err
	}
	// Generate per-model hooks file (e.g. model/user.go)
	hooksData := map[string]any{
		"Name": model.Name,
	}
	return g.executeTemplate("model_hooks.tmpl", filepath.Join(g.outputDir, "model", strings.ToLower(model.Name)+".go"), hooksData)
}

func (g *Generator) generateRepositoryBaseFile() error {
	data := map[string]any{
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("repository_base.tmpl", filepath.Join(g.outputDir, "repository/base.go"), data)
}

func (g *Generator) generateRepositoryFile(model *parser.Model) error {
	data := map[string]any{
		"Model":      model,
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("repository.tmpl", filepath.Join(g.outputDir, "repository", "repository.go"), data)
}

func (g *Generator) copyIDLFile(idlSrcPath string) error {
	dstPath := filepath.Join(g.outputDir, "idl.go")
	src, err := os.ReadFile(idlSrcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, src, 0644)
}

func (g *Generator) executeTemplate(templateName, filePath string, data any) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return g.templates.ExecuteTemplate(f, templateName, data)
}

// Generate is an alias for GenerateFull that accepts only a list of services.
// Models and Source are left at their zero values.
func (g *Generator) Generate(services []*parser.Service) error {
	return g.GenerateFull(&parser.ParseResult{Services: services})
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(r + 32)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
