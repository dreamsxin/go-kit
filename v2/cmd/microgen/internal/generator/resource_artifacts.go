package generator

import (
	"fmt"
	"strings"
)

// resourceTemplateData is what resource.tmpl renders for one table.
//
// The template needs the column each field answers to, the Go type the model
// declares for it, and the converter that bridges the two — everything else it
// can read from the model view.
type resourceTemplateData struct {
	Model      *modelView
	ImportPath string
	Fields     []resourceFieldView
	Primary    resourceFieldView
}

// resourceFieldView is one column as both a resource field and a model field.
type resourceFieldView struct {
	// GoName is the field's name in the generated struct.
	GoName string
	// Column is its name in the table, and therefore in a filter, a sort and a
	// response. Field names are column names on purpose: the schema is derived
	// from the table, so a name can only reach SQL by being declared.
	Column string
	// Type is the Go type without the pointer, which is what decides the
	// converter and the resource field type.
	Type string
	// Pointer reports whether the model declares a pointer, which a nullable
	// column does.
	Pointer bool
	// Converter names the function that turns a JSON value into Type.
	Converter string
	// Declaration is the Go expression that declares this field in a schema.
	Declaration string
	// IsAutoIncr reports whether the database assigns the value.
	IsAutoIncr bool
}

// resourceCollectionsTemplateData is what resource_collections.tmpl renders.
type resourceCollectionsTemplateData struct {
	Models  []*modelView
	Skipped []resourceSkip
}

// resourceSkip records a table that has no declarative surface and why, so the
// omission is a decision the generated project states rather than one a reader
// has to notice.
type resourceSkip struct {
	Table  string
	Reason string
}

// generateResourceArtifacts writes the declarative surface: the shared query
// translation, one declaration per table, and the list the mounting code reads.
func (g *Generator) generateResourceArtifacts(models []*modelView) error {
	if err := g.generateResourceStoreFile(); err != nil {
		return fmt.Errorf("generate resource store failed: %w", err)
	}
	declared, skipped := resourceTables(models)
	for _, model := range declared {
		if err := g.generateResourceFile(model); err != nil {
			return fmt.Errorf("generate resource[%s] failed: %w", model.Name, err)
		}
	}
	if err := g.generateResourceCollectionsFile(declared, skipped); err != nil {
		return fmt.Errorf("generate resource collections failed: %w", err)
	}
	return nil
}

// resourcePrefix is the path the declarative surface mounts under.
//
// It is deliberately not the service prefix: a collection named after its table
// would answer {prefix}/{table}/{id}, which is the pattern a generated endpoint
// already answers. One segment between them keeps the two surfaces addressable.
func resourcePrefix(routePrefix string) string {
	prefix := strings.Trim(strings.TrimSpace(routePrefix), "/")
	if prefix == "" {
		return "/resources"
	}
	return "/" + prefix + "/resources"
}

// generateResourceStoreFile writes the shared query translation.
func (g *Generator) generateResourceStoreFile() error {
	return g.executeTemplate("resource_store.tmpl", g.layout.resourceStoreFile(), nil)
}

// generateResourceFile writes one table's declaration and the three typed
// operations gorm cannot express through a map.
func (g *Generator) generateResourceFile(model *modelView) error {
	data, err := newResourceTemplateData(model, g.config.ImportPath)
	if err != nil {
		return err
	}
	return g.executeTemplate("resource.tmpl", g.layout.resourceFile(model.Name), data)
}

// generateResourceCollectionsFile writes the list the mounting code iterates.
func (g *Generator) generateResourceCollectionsFile(models []*modelView, skipped []resourceSkip) error {
	return g.executeTemplate("resource_collections.tmpl", g.layout.resourceCollectionsFile(),
		resourceCollectionsTemplateData{Models: models, Skipped: skipped})
}

func newResourceTemplateData(model *modelView, importPath string) (resourceTemplateData, error) {
	primary, ok := singlePrimaryKey(model)
	if !ok {
		return resourceTemplateData{}, fmt.Errorf("model %s has no single-column primary key", model.Name)
	}
	data := resourceTemplateData{Model: model, ImportPath: importPath}
	for _, field := range model.Fields {
		if strings.TrimSpace(field.Name) == "" {
			continue
		}
		data.Fields = append(data.Fields, newResourceFieldView(field))
	}
	for _, view := range data.Fields {
		if view.Column == gormColumnName(primary) {
			data.Primary = view
			break
		}
	}
	if data.Primary.Column == "" {
		return resourceTemplateData{}, fmt.Errorf("model %s: primary key %s is not a rendered field", model.Name, primary.Name)
	}
	return data, nil
}

func newResourceFieldView(field modelFieldView) resourceFieldView {
	base := strings.TrimPrefix(field.Type, "*")
	view := resourceFieldView{
		GoName:     field.Name,
		Column:     gormColumnName(field),
		Type:       base,
		Pointer:    strings.HasPrefix(field.Type, "*"),
		Converter:  resourceConverter(base),
		IsAutoIncr: field.IsAutoIncr,
	}
	view.Declaration = resourceDeclaration(view, field)
	return view
}

// resourceDeclaration renders one field's schema entry.
//
// A blob or a document is declared opaque: a response may carry it, and no query
// may name it, because neither has a portable order or a comparison that means
// the same thing on two drivers.
func resourceDeclaration(view resourceFieldView, field modelFieldView) string {
	kind := resourceFieldTypeName(view.Type)
	if kind == "" {
		return fmt.Sprintf("opaque(%q)", view.Column)
	}
	declaration := fmt.Sprintf("readOnly(%q, resource.%s)", view.Column, kind)
	// A key the database assigns is the one column a client may not write: the
	// insert would either be ignored or collide with the sequence.
	if !field.IsPrimary || !field.IsAutoIncr {
		declaration = "writable(" + declaration + ")"
	}
	return declaration
}

// resourceFieldTypeName maps a Go type to the resource field type that names its
// value domain, or "" for a domain a query cannot address.
func resourceFieldTypeName(goType string) string {
	switch goType {
	case "int16", "int", "int64", "uint":
		return "TypeInt"
	case "float32", "float64":
		return "TypeFloat"
	case "bool":
		return "TypeBool"
	case "time.Time":
		return "TypeTime"
	case "[]byte":
		return ""
	default:
		return "TypeString"
	}
}

// resourceConverter names the function that converts a JSON value into one Go
// type. An unknown type falls back to text, which is what the generator maps an
// unrecognised column type to in the first place.
func resourceConverter(goType string) string {
	switch goType {
	case "int16":
		return "columnInt16"
	case "int":
		return "columnInt"
	case "int64":
		return "columnInt64"
	case "uint":
		return "columnUint"
	case "float32":
		return "columnFloat32"
	case "float64":
		return "columnFloat64"
	case "bool":
		return "columnBool"
	case "time.Time":
		return "columnTime"
	case "[]byte":
		return "columnBytes"
	default:
		return "columnString"
	}
}

// resourceTables splits the models into those that can answer a REST route and
// those that cannot.
//
// The contract addresses one record with one path segment, so a table without a
// single-column primary key has no address to give: it is left out, and the
// reason is written into the generated file rather than left to be discovered by
// a client asking for a table that is not there.
func resourceTables(models []*modelView) ([]*modelView, []resourceSkip) {
	included := make([]*modelView, 0, len(models))
	var skipped []resourceSkip
	for _, model := range models {
		keyFields := primaryKeyFields(model)
		switch len(keyFields) {
		case 1:
			included = append(included, model)
		case 0:
			skipped = append(skipped, resourceSkip{
				Table:  model.TableName,
				Reason: "it declares no primary key, so a path cannot address one record",
			})
		default:
			names := make([]string, 0, len(keyFields))
			for _, field := range keyFields {
				names = append(names, gormColumnName(field))
			}
			skipped = append(skipped, resourceSkip{
				Table:  model.TableName,
				Reason: fmt.Sprintf("its primary key is composite (%s), and one path segment names one value", strings.Join(names, ", ")),
			})
		}
	}
	return included, skipped
}

func primaryKeyFields(model *modelView) []modelFieldView {
	var fields []modelFieldView
	for _, field := range model.Fields {
		if field.IsPrimary {
			fields = append(fields, field)
		}
	}
	return fields
}

func singlePrimaryKey(model *modelView) (modelFieldView, bool) {
	fields := primaryKeyFields(model)
	if len(fields) != 1 {
		return modelFieldView{}, false
	}
	return fields[0], true
}
