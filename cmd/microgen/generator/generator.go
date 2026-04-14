package generator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// Options configures generation behavior.
type Options struct {
	TemplateFS  fs.FS
	OutputDir   string
	ImportPath  string
	ServiceName string
	Protocols   []string
	WithConfig  bool
	WithDocs    bool
	WithTests   bool
	WithModel   bool
	WithGRPC    bool
	WithDB      bool
	DBDriver    string
	WithSwag    bool
	WithSkill   bool
	IDLSrcPath  string
	RoutePrefix string
}

// Generator executes project generation from parsed definitions.
type Generator struct {
	config    Options
	templates *template.Template
	outputDir string
	layout    projectLayout
}

// New creates a new generator.
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

	tmpl := newTemplateSet()

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
		layout:    newProjectLayout(absOut),
	}, nil
}

// SvcRoute is a helper for main.tmpl.
type SvcRoute struct {
	Service    *parser.Service
	FullPrefix string
}

// GenerateFull runs the full project generation flow.
func (g *Generator) GenerateFull(result *parser.ParseResult) error {
	if err := g.prepareProject(result); err != nil {
		return err
	}
	if err := g.generateModelArtifacts(result); err != nil {
		return err
	}
	if err := g.generateServiceArtifacts(result); err != nil {
		return err
	}
	return g.generateFinalProjectArtifacts(result)
}

func (g *Generator) createDirStructure(result *parser.ParseResult) error {
	return g.layout.ensureDirs(result, g.config)
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
