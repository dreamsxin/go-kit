package generator

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// protocHint prints instructions for manually running protoc after .proto generation.
func protocHint(svcPkg, absPbDir string) {
	fmt.Printf("\n[protoc] .proto file generated. Run protoc manually to produce pb.go:\n\n")
	fmt.Printf("  # Step 1 - install plugins (first time only)\n")
	fmt.Printf("  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest\n")
	fmt.Printf("  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest\n\n")
	fmt.Printf("  # Step 2 - generate %s\n", svcPkg)
	fmt.Printf("  protoc \\\n")
	fmt.Printf("    --proto_path=%s \\\n", absPbDir)
	fmt.Printf("    --go_out=%s --go_opt=paths=source_relative \\\n", absPbDir)
	fmt.Printf("    --go-grpc_out=%s --go-grpc_opt=paths=source_relative \\\n", absPbDir)
	fmt.Printf("    %s\n\n", filepath.Join(absPbDir, svcPkg+".proto"))
}

// invalidPkgChar 匹配 Go package 名中不合法的字符（非字母、数字、下划线）
var invalidPkgChar = regexp.MustCompile(`[^a-zA-Z0-9_]`)



// DBDriverMeta 数据库驱动元数据
type DBDriverMeta struct {
	// Driver 驱动名称，与 -db.driver 参数对应
	Driver string
	// ImportPkg gorm 驱动包路径
	ImportPkg string
	// OpenCall 生成代码中 gorm.Open 的第一个参数表达式，含占位符 %s 代表 DSN 变量名
	OpenCall string
	// DefaultDSN 默认 DSN 示例（命令行 -db.dsn 的默认值）
	DefaultDSN string
	// ConfigDSN config.yaml 中展示的 DSN 示例
	ConfigDSN string
}

// supportedDrivers 所有受支持的 gorm 驱动
var supportedDrivers = map[string]DBDriverMeta{
	"sqlite": {
		Driver:     "sqlite",
		ImportPkg:  "gorm.io/driver/sqlite",
		OpenCall:   "sqlite.Open(*dsn)",
		DefaultDSN: "app.db",
		ConfigDSN:  "app.db",
	},
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
}

// Options 生成器配置选项
type Options struct {
	TemplateFS  fs.FS    // 模板文件系统（可传入 &embed.FS 或 os.DirFS(...)）
	OutputDir   string    // 输出目录
	ImportPath  string    // 项目导入路径
	ServiceName string    // 服务名称
	Protocols   []string  // 支持的协议 (http, grpc)
	WithConfig  bool      // 是否生成配置文件
	WithDocs    bool      // 是否生成文档（README）
	WithTests   bool      // 是否生成测试文件
	WithModel   bool      // 是否生成 gorm model + repository
	WithGRPC    bool      // 是否生成 gRPC 传输层
	WithDB      bool      // main 是否包含数据库初始化代码
	DBDriver    string    // 数据库驱动: sqlite(默认)/mysql/postgres/sqlserver/clickhouse
	WithSwag    bool      // 是否生成 swaggo 注释 + docs stub + /swagger/ 路由
	IDLSrcPath  string    // IDL 源文件路径（用于复制到输出目录作为 IDL package）
	RoutePrefix string    // HTTP 路由前缀（如 /api/v1），留空时使用 /<servicename>
}

// Generator 代码生成器
type Generator struct {
	outputDir string
	templates *template.Template
	config    Options
}

// New 创建代码生成器实例
func New(opts Options) (*Generator, error) {
	// ── 驱动默认值 & 校验 ──
	if opts.DBDriver == "" {
		opts.DBDriver = "sqlite"
	}
	opts.DBDriver = strings.ToLower(strings.TrimSpace(opts.DBDriver))
	if _, ok := supportedDrivers[opts.DBDriver]; !ok {
		supported := make([]string, 0, len(supportedDrivers))
		for k := range supportedDrivers {
			supported = append(supported, k)
		}
		return nil, fmt.Errorf("unsupported db driver %q, allowed: %s", opts.DBDriver, strings.Join(supported, ", "))
	}

	// ── 协议检测 ──
	for _, p := range opts.Protocols {
		if strings.TrimSpace(p) == "grpc" {
			opts.WithGRPC = true
		}
	}

	filenames, err := fs.Glob(opts.TemplateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to find template files: %v", err)
	}

	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     strings.Title,
		"trimStar":  func(s string) string { return strings.TrimPrefix(s, "*") },
		"hasPrefix": strings.HasPrefix,
		"join":      strings.Join,
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(opts.TemplateFS, filenames...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %v", err)
	}

	return &Generator{
		outputDir: opts.OutputDir,
		templates: tmpl,
		config:    opts,
	}, nil
}

// Generate 根据解析结果生成代码
func (g *Generator) Generate(services []*parser.Service) error {
	return g.GenerateFull(&parser.ParseResult{
		Services: services,
	})
}

// GenerateFull 根据完整解析结果生成代码（包含 model）
func (g *Generator) GenerateFull(result *parser.ParseResult) error {
	// 创建目录结构
	if err := g.createDirStructure(result); err != nil {
		return err
	}

	// 生成 go.mod（仅在目录中不存在时生成）
	if g.config.ImportPath != "" {
		if err := g.generateGoMod(); err != nil {
			return fmt.Errorf("generate go.mod failed: %w", err)
		}
	}

	// 复制 IDL 文件到输出目录根（作为 IDL package，供生成代码 import）
	if g.config.IDLSrcPath != "" {
		if err := g.copyIDLFile(g.config.IDLSrcPath); err != nil {
			return fmt.Errorf("copy idl file failed: %w", err)
		}
	}

	// 生成 model + repository
	if g.config.WithModel && len(result.Models) > 0 {
		if err := g.generateModelFile(result); err != nil {
			return fmt.Errorf("generate model failed: %w", err)
		}
		if err := g.generateRepositoryFile(result); err != nil {
			return fmt.Errorf("generate repository failed: %w", err)
		}
	} else {
		// 若本次不生成 model/repository，清理旧产物（避免旧文件中的错误 import 干扰 go build/tidy）
		g.removeStaleGoFiles(filepath.Join(g.outputDir, "model"))
		g.removeStaleGoFiles(filepath.Join(g.outputDir, "repository"))
	}

	// 为每个服务生成代码
	for _, service := range result.Services {
		if err := g.generateServiceFileFull(service, result.Models); err != nil {
			return fmt.Errorf("generate service[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateEndpointsFile(service); err != nil {
			return fmt.Errorf("generate endpoints[%s] failed: %w", service.ServiceName, err)
		}
		if err := g.generateHTTPTransportFile(service); err != nil {
			return fmt.Errorf("generate http transport[%s] failed: %w", service.ServiceName, err)
		}
		if g.config.WithGRPC {
			if err := g.generateGRPCTransportFile(service); err != nil {
				return fmt.Errorf("generate grpc transport[%s] failed: %w", service.ServiceName, err)
			}
			if err := g.generateProtoFile(service); err != nil {
				return fmt.Errorf("generate proto[%s] failed: %w", service.ServiceName, err)
			}
		}
		if g.config.WithTests {
			if err := g.generateTestFile(service); err != nil {
				return fmt.Errorf("generate test[%s] failed: %w", service.ServiceName, err)
			}
		}
		if err := g.generateClientDemo(service); err != nil {
			return fmt.Errorf("generate client[%s] failed: %w", service.ServiceName, err)
		}
	}

	// 生成 main 文件
	if err := g.generateMainFileFull(result.Services, result.Models); err != nil {
		return fmt.Errorf("generate main failed: %w", err)
	}

	// 生成配置文件（config.yaml + config.go）
	if g.config.WithConfig {
		if err := g.generateConfigFile(result.Services); err != nil {
			return fmt.Errorf("generate config.yaml failed: %w", err)
		}
		if err := g.generateConfigCodeFile(result.Services); err != nil {
			return fmt.Errorf("generate config.go failed: %w", err)
		}
	}

	// 生成文档
	if g.config.WithDocs {
		if err := g.generateReadme(result.Services); err != nil {
			return fmt.Errorf("generate readme failed: %w", err)
		}
	}

	// 生成 swag docs stub（占位文件，运行 swag init 后会被覆盖）
	if g.config.WithSwag {
		if err := g.generateDocsStub(result.Services); err != nil {
			return fmt.Errorf("generate docs stub failed: %w", err)
		}
	}

	return nil
}

// ─────────────────────────── 目录结构 ───────────────────────────

func (g *Generator) createDirStructure(result *parser.ParseResult) error {
	// 只创建真正会写入文件的目录
	dirs := []string{
		filepath.Join(g.outputDir, "cmd"),
		filepath.Join(g.outputDir, "service"),
		filepath.Join(g.outputDir, "endpoint"),
		filepath.Join(g.outputDir, "transport"),
	}

	// client demo 始终生成
	dirs = append(dirs, filepath.Join(g.outputDir, "client"))

	// config.yaml
	if g.config.WithConfig {
		dirs = append(dirs, filepath.Join(g.outputDir, "config"))
	}

	// gorm model + repository
	if g.config.WithModel {
		dirs = append(dirs,
			filepath.Join(g.outputDir, "model"),
			filepath.Join(g.outputDir, "repository"),
		)
	}

	// gRPC: pb/<svcname>/
	if g.config.WithGRPC {
		for _, svc := range result.Services {
			dirs = append(dirs,
				filepath.Join(g.outputDir, "pb", svc.PackageName),
			)
		}
	}

	// 测试文件
	if g.config.WithTests {
		dirs = append(dirs, filepath.Join(g.outputDir, "test"))
	}

	// swag 文档 stub
	if g.config.WithSwag {
		dirs = append(dirs, filepath.Join(g.outputDir, "docs"))
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	return nil
}

// ─────────────────────────── Model / Repository ───────────────────────────

func (g *Generator) generateModelFile(result *parser.ParseResult) error {
	// 过滤有 gorm tag 的 model
	var gormModels []*parser.Model
	for _, m := range result.Models {
		if m.HasGormTags {
			gormModels = append(gormModels, m)
		}
	}
	if len(gormModels) == 0 {
		// 没有显式 gorm tag 也生成（全部 struct）
		gormModels = result.Models
	}

	data := map[string]interface{}{
		"Models":     gormModels,
		"ImportPath": g.config.ImportPath,
	}
	filePath := filepath.Join(g.outputDir, "model", "model.go")
	return g.executeTemplate("model.tmpl", filePath, data)
}

func (g *Generator) generateRepositoryFile(result *parser.ParseResult) error {
	var models []*parser.Model
	for _, m := range result.Models {
		if m.HasGormTags {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		models = result.Models
	}

	data := map[string]interface{}{
		"Models":     models,
		"ImportPath": g.config.ImportPath,
	}
	filePath := filepath.Join(g.outputDir, "repository", "repository.go")
	return g.executeTemplate("repository.tmpl", filePath, data)
}

// ─────────────────────────── 服务层 ───────────────────────────

func (g *Generator) generateServiceFile(service *parser.Service) error {
	return g.generateServiceFileFull(service, nil)
}

func (g *Generator) generateServiceFileFull(service *parser.Service, models []*parser.Model) error {
	serviceDir := filepath.Join(g.outputDir, "service", service.PackageName)
	os.MkdirAll(serviceDir, 0755)

	// 过滤出有 gorm tag 的 model（与 generateModelFile 保持一致）
	var gormModels []*parser.Model
	for _, m := range models {
		if m.HasGormTags {
			gormModels = append(gormModels, m)
		}
	}
	if len(models) > 0 && len(gormModels) == 0 {
		gormModels = models
	}

	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"WithModel":  g.config.WithModel,
		"GormModels": gormModels,
	}
	return g.executeTemplate("service.tmpl", filepath.Join(serviceDir, "service.go"), data)
}

// ─────────────────────────── 端点层 ───────────────────────────

func (g *Generator) generateEndpointsFile(service *parser.Service) error {
	endpointDir := filepath.Join(g.outputDir, "endpoint", service.PackageName)
	os.MkdirAll(endpointDir, 0755)
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("endpoints.tmpl", filepath.Join(endpointDir, "endpoints.go"), data)
}

// ─────────────────────────── HTTP Transport ───────────────────────────

func (g *Generator) generateHTTPTransportFile(service *parser.Service) error {
	transportDir := filepath.Join(g.outputDir, "transport", service.PackageName)
	os.MkdirAll(transportDir, 0755)

	// 完整路由前缀 = -prefix + /svcname（transport 注释用）
	fullPrefix := g.config.RoutePrefix + "/" + service.PackageName

	data := map[string]interface{}{
		"Service":     service,
		"ImportPath":  g.config.ImportPath,
		"RoutePrefix": fullPrefix,
	}
	return g.executeTemplate("transport.tmpl", filepath.Join(transportDir, "transport_http.go"), data)
}

// ─────────────────────────── gRPC Transport ───────────────────────────

func (g *Generator) generateGRPCTransportFile(service *parser.Service) error {
	transportDir := filepath.Join(g.outputDir, "transport", service.PackageName)
	os.MkdirAll(transportDir, 0755)
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("transport_grpc.tmpl", filepath.Join(transportDir, "transport_grpc.go"), data)
}

// generateProtoFile 生成 .proto 文件，并输出手动执行 protoc 的提示。
// 不自动调用 protoc，由用户在生成完成后根据提示手动执行。
func (g *Generator) generateProtoFile(service *parser.Service) error {
	pbDir := filepath.Join(g.outputDir, "pb", service.PackageName)
	os.MkdirAll(pbDir, 0755)
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}
	protoFile := filepath.Join(pbDir, service.PackageName+".proto")
	if err := g.executeTemplate("proto.tmpl", protoFile, data); err != nil {
		return err
	}
	// 转成绝对路径，供提示信息使用
	absPbDir, err := filepath.Abs(pbDir)
	if err != nil {
		absPbDir = pbDir
	}
	protocHint(service.PackageName, absPbDir)
	return nil
}

// ─────────────────────────── Client Demo ───────────────────────────

func (g *Generator) generateClientDemo(service *parser.Service) error {
	clientDir := filepath.Join(g.outputDir, "client", service.PackageName)
	os.MkdirAll(clientDir, 0755)
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
		"WithGRPC":   g.config.WithGRPC,
	}
	return g.executeTemplate("client.tmpl", filepath.Join(clientDir, "demo.go"), data)
}

// ─────────────────────────── 测试文件 ───────────────────────────

func (g *Generator) generateTestFile(service *parser.Service) error {
	data := map[string]interface{}{
		"Service":    service,
		"ImportPath": g.config.ImportPath,
	}
	return g.executeTemplate("service_test.tmpl",
		filepath.Join(g.outputDir, "test", service.PackageName+"_test.go"), data)
}

// ─────────────────────────── Main 文件 ───────────────────────────

func (g *Generator) generateMainFile(services []*parser.Service) error {
	return g.generateMainFileFull(services, nil)
}

func (g *Generator) generateMainFileFull(services []*parser.Service, models []*parser.Model) error {
	meta := supportedDrivers[g.config.DBDriver]
	// 用第一个服务名替换 DSN 中的 {svcname} 占位符
	svcName := ""
	if len(services) > 0 {
		svcName = strings.ToLower(services[0].ServiceName)
	}
	defaultDSN := strings.ReplaceAll(meta.DefaultDSN, "{svcname}", svcName)

	// 过滤出有 gorm tag 的 model（需要 AutoMigrate）
	var gormModels []*parser.Model
	for _, m := range models {
		if m.HasGormTags {
			gormModels = append(gormModels, m)
		}
	}

	// 计算每个服务的路由前缀：
	//   RoutePrefix  = -prefix 参数（如 /api/v1），留空时为 ""
	//   FullPrefix   = RoutePrefix + /svcname（完整挂载点，如 /api/v1/userservice 或 /userservice）
	type svcRoute struct {
		Service     *parser.Service
		RoutePrefix string // 纯版本前缀（可为空）
		FullPrefix  string // 完整挂载前缀 = RoutePrefix + /svcname
	}
	svcRoutes := make([]svcRoute, 0, len(services))
	for _, svc := range services {
		rp := g.config.RoutePrefix
		fp := rp + "/" + svc.PackageName
		svcRoutes = append(svcRoutes, svcRoute{Service: svc, RoutePrefix: rp, FullPrefix: fp})
	}

	data := map[string]interface{}{
		"Services":     services,
		"SvcRoutes":    svcRoutes,
		"ImportPath":   g.config.ImportPath,
		"NeedMux":      len(services) > 1,
		"WithGRPC":     g.config.WithGRPC,
		"WithDB":       g.config.WithDB || g.config.WithModel,
		"DBDriver":     meta.Driver,
		"DBImportPkg":  meta.ImportPkg,
		"DBOpenCall":   meta.OpenCall,
		"DBDefaultDSN": defaultDSN,
		"WithSwag":     g.config.WithSwag,
		"WithConfig":   g.config.WithConfig,
		"GormModels":   gormModels,
	}
	return g.executeTemplate("main.tmpl", filepath.Join(g.outputDir, "cmd", "main.go"), data)
}

// ─────────────────────────── Config 文件 ───────────────────────────

// configTemplateData 构建 config.tmpl / config_code.tmpl 所需的模板数据
func (g *Generator) configTemplateData(services []*parser.Service) map[string]interface{} {
	meta := supportedDrivers[g.config.DBDriver]
	svcName := ""
	if len(services) > 0 {
		svcName = strings.ToLower(services[0].ServiceName)
	}
	configDSN := strings.ReplaceAll(meta.ConfigDSN, "{svcname}", svcName)
	defaultDSN := strings.ReplaceAll(meta.DefaultDSN, "{svcname}", svcName)
	serviceName := ""
	if len(services) > 0 {
		serviceName = services[0].ServiceName
	}
	return map[string]interface{}{
		"ServiceName":  serviceName,
		"WithGRPC":     g.config.WithGRPC,
		"WithDB":       g.config.WithDB || g.config.WithModel,
		"WithSwag":     g.config.WithSwag,
		"DBDriver":     meta.Driver,
		"DBConfigDSN":  configDSN,
		"DBDefaultDSN": defaultDSN,
	}
}

// generateConfigFile 生成 config/config.yaml
func (g *Generator) generateConfigFile(services []*parser.Service) error {
	if len(services) == 0 {
		return nil
	}
	data := g.configTemplateData(services)
	return g.executeTemplate("config.tmpl", filepath.Join(g.outputDir, "config", "config.yaml"), data)
}

// generateConfigCodeFile 生成 config/config.go（Go struct + Load 函数）
func (g *Generator) generateConfigCodeFile(services []*parser.Service) error {
	if len(services) == 0 {
		return nil
	}
	data := g.configTemplateData(services)
	return g.executeTemplate("config_code.tmpl", filepath.Join(g.outputDir, "config", "config.go"), data)
}

// ─────────────────────────── README ───────────────────────────

func (g *Generator) generateReadme(services []*parser.Service) error {
	// 计算每个服务的完整路由前缀（与 generateMainFileFull 保持一致）
	type svcRoute struct {
		Service    *parser.Service
		FullPrefix string // RoutePrefix + /svcname
	}
	svcRoutes := make([]svcRoute, 0, len(services))
	for _, svc := range services {
		svcRoutes = append(svcRoutes, svcRoute{
			Service:    svc,
			FullPrefix: g.config.RoutePrefix + "/" + svc.PackageName,
		})
	}

	// 第一个服务名（用于标题）
	firstName := ""
	if len(services) > 0 {
		firstName = services[0].ServiceName
	}
	if len(services) > 1 {
		firstName = "Multi-Service"
	}

	data := map[string]interface{}{
		"Services":    services,
		"SvcRoutes":   svcRoutes,
		"FirstName":   firstName,
		"ImportPath":  g.config.ImportPath,
		"WithGRPC":    g.config.WithGRPC,
		"WithModel":   g.config.WithModel,
		"WithSwag":    g.config.WithSwag,
		"WithDB":      g.config.WithDB || g.config.WithModel,
		"WithConfig":  g.config.WithConfig,
		"WithTests":   g.config.WithTests,
		"RoutePrefix": g.config.RoutePrefix,
	}

	funcMap := template.FuncMap{
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}

	readmeTmpl := `# {{.FirstName}} 微服务

## 项目结构

` + "```" + `
.
├── cmd/main.go          # 服务入口
├── service/             # 业务逻辑层{{range .Services}}
│   └── {{.PackageName}}/service.go{{end}}
├── endpoint/            # 端点层（熔断、限流、重试）{{range .Services}}
│   └── {{.PackageName}}/endpoints.go{{end}}
├── transport/           # 传输层（HTTP{{if .WithGRPC}} / gRPC{{end}}）{{range .Services}}
│   └── {{.PackageName}}/transport_http.go{{if $.WithGRPC}}
│   └── {{.PackageName}}/transport_grpc.go{{end}}{{end}}
{{- if .WithGRPC}}
├── pb/                  # Protobuf 定义 + 自动生成的 pb.go{{range .Services}}
│   └── {{.PackageName}}/{{.PackageName}}.proto{{end}}
{{- end}}
{{- if .WithModel}}
├── model/model.go       # GORM 数据模型
├── repository/repository.go  # 数据访问层
{{- end}}
├── client/              # 客户端示例{{range .Services}}
│   └── {{.PackageName}}/demo.go{{end}}
{{- if .WithConfig}}
├── config/config.yaml   # 服务配置
├── config/config.go     # 配置加载
{{- end}}
{{- if .WithTests}}
├── test/                # 单元测试
{{- end}}
{{- if .WithSwag}}
├── docs/                # Swagger 文档（swag init 生成）
{{- end}}
└── idl.go               # 服务接口定义（IDL）
` + "```" + `

## API 端点

{{- range .SvcRoutes}}
{{- $svc := .}}

### {{.Service.ServiceName}}

| Method | Path | Description |
|--------|------|-------------|
{{- range .Service.Methods}}
| ` + "`" + `{{.HTTPMethod | upper}}` + "`" + ` | ` + "`" + `{{$svc.FullPrefix}}{{.Route}}` + "`" + ` | {{.Doc}} |
{{- end}}
{{- end}}
{{if .WithSwag}}
> 启动后访问 Swagger UI：<http://localhost:8080/swagger/index.html>
{{end}}
{{- if .WithGRPC}}
## gRPC 服务

.proto 文件已生成到 pb/ 目录，**需要手动执行 protoc** 生成 pb.go：

` + "```" + `bash
# 1. 安装插件（仅首次）
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
{{range .Services}}
# 2. 生成 {{.ServiceName}}
protoc \
  --proto_path=pb/{{.PackageName}} \
  --go_out=pb/{{.PackageName}} --go_opt=paths=source_relative \
  --go-grpc_out=pb/{{.PackageName}} --go-grpc_opt=paths=source_relative \
  pb/{{.PackageName}}/{{.PackageName}}.proto
{{end}}` + "```" + `
{{end}}
{{- if .WithDB}}
## 数据库

默认使用配置文件中的 DSN，启动时通过 -db.dsn 参数覆盖。
{{if .WithModel}}AutoMigrate 会在首次启动时自动建表，无需手动执行 DDL。{{end}}
{{end}}
## 快速启动

` + "```" + `bash
# 整理依赖
go mod tidy

# 启动服务
go run ./cmd/main.go \
  -http.addr :8080{{if .WithGRPC}} \
  -grpc.addr :8081{{end}}{{if .WithDB}} \
  -db.dsn "your-dsn"{{end}}
` + "```" + `

## 健康检查

` + "```" + `bash
curl http://localhost:8080/health
` + "```" + `
{{- if .WithSwag}}

## 生成 Swagger 文档

` + "```" + `bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/main.go -o docs
` + "```" + `
{{- end}}
`

	tmpl, err := template.New("readme").Funcs(funcMap).Parse(readmeTmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(g.outputDir, "README.md"))
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

// ─────────────────────────── Swag Docs Stub ───────────────────────────

// stubMarker 是占位文件的特征字符串，用于判断 docs.go 是否仍是未初始化的 stub。
const stubMarker = `"paths": {}`

// generateDocsStub 生成 docs/docs.go 占位文件。
// 仅在文件不存在或仍是 stub（含 "paths": {}）时才写入，避免覆盖 swag init 的结果。
// 写入后自动尝试运行 swag init 生成真实文档。
func (g *Generator) generateDocsStub(services []*parser.Service) error {
	title := "API"
	description := ""
	basePath := "/"
	if len(services) > 0 {
		title = services[0].Title
		description = services[0].Description
		basePath = "/" + services[0].PackageName
	}

	docsFile := filepath.Join(g.outputDir, "docs", "docs.go")

	// 若已存在真实文档（不含 stub 标记），保留不覆盖
	if existing, err := os.ReadFile(docsFile); err == nil {
		if !bytes.Contains(existing, []byte(stubMarker)) {
			// 已是 swag init 生成的真实文档，直接运行 swag init 刷新即可
			return g.runSwagInit()
		}
	}

	stub := fmt.Sprintf(`// Code generated by microgen. DO NOT EDIT.
// Run "swag init -g cmd/main.go" to regenerate.
//
// swag docs stub — this file will be overwritten by "swag init".
// Install swag: go install github.com/swaggo/swag/cmd/swag@latest

package docs

import "github.com/swaggo/swag"

const docTemplate = `+"`"+`{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{.Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {}
}`+"`"+`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:8080",
	BasePath:         "%s",
	Schemes:          []string{},
	Title:            "%s",
	Description:      "%s",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
`, basePath, title, description)

	if err := os.WriteFile(docsFile, []byte(stub), 0644); err != nil {
		return err
	}

	// stub 写入后立即尝试运行 swag init，让文档立刻生效
	return g.runSwagInit()
}

// runSwagInit 在输出目录执行 swag init，生成真实的 Swagger 文档。
// 若 swag 工具未安装，只打印提示而不返回错误，保证 microgen 主流程不中断。
func (g *Generator) runSwagInit() error {
	swagBin, err := exec.LookPath("swag")
	if err != nil {
		// swag 未安装，提示用户手动运行
		fmt.Printf("[warn] swag not found in PATH; run manually:\n  cd %s && swag init -g cmd/main.go -o docs\n", g.outputDir)
		return nil
	}

	cmd := exec.Command(swagBin, "init", "-g", "cmd/main.go", "-o", "docs")
	cmd.Dir = g.outputDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// swag init 失败不是致命错误，打印输出供调试
		fmt.Printf("[warn] swag init failed (non-fatal):\n%s\n", string(out))
		return nil
	}
	fmt.Printf("[ok] swag init completed\n")
	return nil
}

// ─────────────────────────── go.mod ───────────────────────────

// generateGoMod 在输出目录中生成 go.mod。
// - 若文件不存在：全量写入。
// - 若文件已存在但 module 名与 ImportPath 不符：仅更新 module 行，保留其余内容（依赖等不丢失）。
// - 若文件已存在且 module 名一致：跳过，避免覆盖用户修改。
func (g *Generator) generateGoMod() error {
	goModPath := filepath.Join(g.outputDir, "go.mod")
	existing, err := os.ReadFile(goModPath)
	if err != nil {
		// 文件不存在，全量写入（含完整 require 块）
		content := g.buildGoModContent()
		return os.WriteFile(goModPath, []byte(content), 0644)
	}

	// 文件已存在：检查 module 行是否一致
	lines := strings.Split(string(existing), "\n")
	wantModule := "module " + g.config.ImportPath
	if strings.TrimSpace(lines[0]) == wantModule {
		// 已一致，跳过（保留用户修改）
		return nil
	}

	// module 名不一致，仅替换第一行，保留其余内容（require 块等不丢失）
	lines[0] = wantModule
	return os.WriteFile(goModPath, []byte(strings.Join(lines, "\n")), 0644)
}

// buildGoModContent 根据生成选项构建完整的 go.mod 内容。
// 包含所有可能用到的依赖，go mod tidy 会自动裁剪未使用的部分。
func (g *Generator) buildGoModContent() string {
	var sb strings.Builder
	sb.WriteString("module " + g.config.ImportPath + "\n\n")
	sb.WriteString("go 1.21\n\n")
	sb.WriteString("require (\n")
	// go-kit 核心（本地 replace）
	sb.WriteString("\tgithub.com/dreamsxin/go-kit v0.0.0\n")
	// HTTP 传输层
	sb.WriteString("\tgithub.com/gorilla/mux v1.8.1\n")
	sb.WriteString("\tgithub.com/gorilla/schema v1.2.1\n")
	// gRPC
	if g.config.WithGRPC {
		sb.WriteString("\tgoogle.golang.org/grpc v1.61.0\n")
		sb.WriteString("\tgoogle.golang.org/protobuf v1.31.0\n")
	}
	// GORM model
	if g.config.WithModel {
		switch g.config.DBDriver {
		case "mysql":
			sb.WriteString("\tgorm.io/driver/mysql v1.5.2\n")
		case "postgres":
			sb.WriteString("\tgorm.io/driver/postgres v1.5.4\n")
		case "sqlserver":
			sb.WriteString("\tgorm.io/driver/sqlserver v1.5.2\n")
		case "clickhouse":
			sb.WriteString("\tgorm.io/driver/clickhouse v0.6.0\n")
		default: // sqlite
			sb.WriteString("\tgorm.io/driver/sqlite v1.5.4\n")
		}
		sb.WriteString("\tgorm.io/gorm v1.25.5\n")
	}
	// Swagger
	if g.config.WithSwag {
		sb.WriteString("\tgithub.com/swaggo/http-swagger v1.3.4\n")
		sb.WriteString("\tgithub.com/swaggo/swag v1.16.3\n")
	}
	// config yaml
	sb.WriteString("\tgopkg.in/yaml.v3 v3.0.1\n")
	sb.WriteString(")\n\n")
	sb.WriteString("replace github.com/dreamsxin/go-kit => ../\n")
	return sb.String()
}

// copyIDLFile 将 IDL 源文件复制到输出目录根（作为 IDL package），
// 并将 package 声明替换为根 package 名（取 import path 最后一段）。
func (g *Generator) copyIDLFile(idlSrcPath string) error {
	// 目标文件名固定为 idl.go
	dstPath := filepath.Join(g.outputDir, "idl.go")

	src, err := os.ReadFile(idlSrcPath)
	if err != nil {
		return fmt.Errorf("read idl file %s: %w", idlSrcPath, err)
	}

	// 推导根 package 名（import path 最后一段）
	// Go package 名只允许字母、数字、下划线，且不能以数字开头
	// import path 末段可能含 - (如 usersvc-grpc)，需替换为 _
	parts := strings.Split(strings.TrimRight(g.config.ImportPath, "/"), "/")
	rawPkg := parts[len(parts)-1]
	rootPkg := invalidPkgChar.ReplaceAllString(rawPkg, "_")

	// 替换 package 声明（第一行 `package xxx`）
	content := string(src)
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) >= 1 && strings.HasPrefix(strings.TrimSpace(lines[0]), "package ") {
		lines[0] = "package " + rootPkg
		content = strings.Join(lines, "\n")
	}

	return os.WriteFile(dstPath, []byte(content), 0644)
}

// ─────────────────────────── 模板执行 ───────────────────────────

func (g *Generator) executeTemplate(templateName, filePath string, data interface{}) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filePath, err)
	}
	defer f.Close()

	return g.templates.ExecuteTemplate(f, templateName, data)
}

// removeStaleGoFiles 删除目录下所有 .go 文件（用于清理不再需要的旧产物）。
// 目录不存在时静默忽略。
func (g *Generator) removeStaleGoFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // 目录不存在或无法读取，忽略
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
