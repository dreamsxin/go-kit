package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
)

// Options configures generation behavior.
type Options struct {
	TemplateFS           fs.FS
	OutputDir            string
	ImportPath           string
	GoKitVersion         string
	ServiceName          string
	Protocols            []string
	WithConfig           bool
	ConfigMode           string
	RemoteProvider       string
	WithDocs             bool
	WithTests            bool
	WithModel            bool
	WithGRPC             bool
	WithDB               bool
	DBDriver             string
	WithOpenAPI          bool
	WithSkill            bool
	WithInteraction      bool
	IDLSrcPath           string
	RoutePrefix          string
	GeneratedMiddlewares []string
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
	opt = opt.Normalize()
	absOut, err := filepath.Abs(opt.OutputDir)
	if err != nil {
		return nil, err
	}

	if err := opt.Validate(); err != nil {
		return nil, err
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
	Service    *ir.Service
	FullPrefix string
}

// GenerateIR runs generation directly from the shared IR.
func (g *Generator) GenerateIR(project *ir.Project) error {
	ctx := newGenerationContext(project)
	if err := g.prepareProject(ctx); err != nil {
		return err
	}
	if err := g.generateModelArtifacts(ctx); err != nil {
		return err
	}
	if err := g.generateServiceArtifacts(ctx); err != nil {
		return err
	}
	return g.generateFinalProjectArtifacts(ctx)
}

func (g *Generator) createDirStructure(services []*serviceView) error {
	return g.layout.ensureDirs(services, g.config)
}

func (g *Generator) executeTemplate(templateName, filePath string, data any) error {
	var rendered bytes.Buffer
	if err := g.templates.ExecuteTemplate(&rendered, templateName, data); err != nil {
		return fmt.Errorf("render %s: %w", templateName, err)
	}

	content := rendered.Bytes()
	if filepath.Ext(filePath) == ".go" {
		formatted, err := format.Source(content)
		if err != nil {
			return fmt.Errorf("format generated Go file %s: %w", filePath, err)
		}
		content = formatted
	} else {
		content = normalizeGeneratedText(content)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return fmt.Errorf("write generated file %s: %w", filePath, err)
	}
	return nil
}

func normalizeGeneratedText(content []byte) []byte {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	text = strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if text == "" {
		return nil
	}
	return []byte(text + "\n")
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
