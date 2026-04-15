package ir

import (
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/dbschema"
)

// FromTableSchemas converts database schema introspection output into the
// source-agnostic microgen IR without going through parser.ParseResult.
func FromTableSchemas(schemas []*dbschema.TableSchema, serviceName, pkgName string) *Project {
	if pkgName == "" {
		pkgName = strings.ToLower(serviceName)
	}

	project := &Project{
		PackageName: pkgName,
		Source:      "db",
	}

	messageByName := map[string]*Message{}
	for _, schema := range schemas {
		model := dbTableToMessage(schema)
		project.Messages = append(project.Messages, model)
		messageByName[model.Name] = model

		for _, dto := range dbCRUDMessages(schema, model.Name) {
			project.Messages = append(project.Messages, dto)
			messageByName[dto.Name] = dto
		}
	}

	svc := &Service{
		Name:        serviceName,
		PackageName: strings.ToLower(serviceName),
		Title:       serviceName + " API",
		Description: "Auto-generated RESTful API from database schema",
	}

	for _, schema := range schemas {
		modelName := singularModelName(schema.TableName)
		for _, method := range dbCRUDMethods(modelName) {
			method.Input = messageByName[method.InputName]
			method.Output = messageByName[method.OutputName]
			svc.Methods = append(svc.Methods, method)
		}
	}
	project.Services = append(project.Services, svc)

	return project
}

func dbTableToMessage(schema *dbschema.TableSchema) *Message {
	modelName := singularModelName(schema.TableName)
	msg := &Message{
		Name:        modelName,
		TableName:   schema.TableName,
		HasGormTags: true,
	}
	for _, col := range schema.Columns {
		if isGormAutoField(col.Name) {
			continue
		}
		goType := dbColumnGoType(col)
		msg.Fields = append(msg.Fields, &Field{
			Name:        dbschema.SnakeToCamel(col.Name),
			JSONName:    col.Name,
			GoType:      goType,
			SchemaType:  goTypeToSchemaType(goType),
			GormTag:     dbColumnGormTag(col),
			Description: col.Comment,
			Required:    !col.IsNullable || col.IsPrimary,
			IsPrimary:   col.IsPrimary,
			IsAutoIncr:  col.IsAutoIncr,
			IsUnique:    col.IsUnique,
		})
	}
	return msg
}

func dbCRUDMessages(schema *dbschema.TableSchema, modelName string) []*Message {
	fields := dbMessageFields(schema)
	pkFields := dbPrimaryKeyFields(schema)
	nonPKFields := dbNonPrimaryFields(fields)

	plural := modelName + "s"
	return []*Message{
		{
			Name:   "Create" + modelName + "Request",
			Fields: cloneFields(filterCreateFields(nonPKFields)),
		},
		{
			Name: "Create" + modelName + "Response",
			Fields: []*Field{
				{Name: modelName, JSONName: strings.ToLower(modelName), GoType: "*" + modelName, SchemaType: "object"},
				{Name: "Error", JSONName: "error", GoType: "string", SchemaType: "string"},
			},
		},
		{
			Name:   "Get" + modelName + "Request",
			Fields: cloneFields(fallbackPrimaryFields(pkFields)),
		},
		{
			Name: "Get" + modelName + "Response",
			Fields: []*Field{
				{Name: modelName, JSONName: strings.ToLower(modelName), GoType: "*" + modelName, SchemaType: "object"},
				{Name: "Error", JSONName: "error", GoType: "string", SchemaType: "string"},
			},
		},
		{
			Name:   "Update" + modelName + "Request",
			Fields: cloneFields(append(fallbackPrimaryFields(pkFields), filterCreateFields(nonPKFields)...)),
		},
		{
			Name: "Update" + modelName + "Response",
			Fields: []*Field{
				{Name: modelName, JSONName: strings.ToLower(modelName), GoType: "*" + modelName, SchemaType: "object"},
				{Name: "Error", JSONName: "error", GoType: "string", SchemaType: "string"},
			},
		},
		{
			Name:   "Delete" + modelName + "Request",
			Fields: cloneFields(fallbackPrimaryFields(pkFields)),
		},
		{
			Name: "Delete" + modelName + "Response",
			Fields: []*Field{
				{Name: "Success", JSONName: "success", GoType: "bool", SchemaType: "boolean", Required: true},
				{Name: "Error", JSONName: "error", GoType: "string", SchemaType: "string"},
			},
		},
		{
			Name: "List" + plural + "Request",
			Fields: []*Field{
				{Name: "Page", JSONName: "page", GoType: "int", SchemaType: "integer", Required: true},
				{Name: "PageSize", JSONName: "page_size", GoType: "int", SchemaType: "integer", Required: true},
			},
		},
		{
			Name: "List" + plural + "Response",
			Fields: []*Field{
				{Name: plural, JSONName: strings.ToLower(plural), GoType: "[]*" + modelName, SchemaType: "array"},
				{Name: "Total", JSONName: "total", GoType: "int", SchemaType: "integer", Required: true},
			},
		},
	}
}

func dbCRUDMethods(modelName string) []*Method {
	lower := strings.ToLower(modelName)
	plural := modelName + "s"
	return []*Method{
		{
			Name:        "Create" + modelName,
			Summary:     "Create " + modelName,
			Description: "Create " + modelName,
			HTTPMethod:  "POST",
			Route:       "/" + lower,
			Tags:        []string{modelName},
			InputName:   "Create" + modelName + "Request",
			OutputName:  "Create" + modelName + "Response",
		},
		{
			Name:        "Get" + modelName,
			Summary:     "Get " + modelName,
			Description: "Get " + modelName + " details",
			HTTPMethod:  "GET",
			Route:       "/" + lower + "/{id}",
			Tags:        []string{modelName},
			InputName:   "Get" + modelName + "Request",
			OutputName:  "Get" + modelName + "Response",
		},
		{
			Name:        "Update" + modelName,
			Summary:     "Update " + modelName,
			Description: "Update " + modelName,
			HTTPMethod:  "PUT",
			Route:       "/" + lower + "/{id}",
			Tags:        []string{modelName},
			InputName:   "Update" + modelName + "Request",
			OutputName:  "Update" + modelName + "Response",
		},
		{
			Name:        "Delete" + modelName,
			Summary:     "Delete " + modelName,
			Description: "Delete " + modelName,
			HTTPMethod:  "DELETE",
			Route:       "/" + lower + "/{id}",
			Tags:        []string{modelName},
			InputName:   "Delete" + modelName + "Request",
			OutputName:  "Delete" + modelName + "Response",
		},
		{
			Name:        "List" + plural,
			Summary:     "List " + plural,
			Description: "List " + plural,
			HTTPMethod:  "GET",
			Route:       "/" + lower + "s",
			Tags:        []string{modelName},
			InputName:   "List" + plural + "Request",
			OutputName:  "List" + plural + "Response",
		},
	}
}

func dbMessageFields(schema *dbschema.TableSchema) []*Field {
	out := make([]*Field, 0, len(schema.Columns))
	for _, col := range schema.Columns {
		if isGormAutoField(col.Name) {
			continue
		}
		goType := dbColumnGoType(col)
		out = append(out, &Field{
			Name:        dbschema.SnakeToCamel(col.Name),
			JSONName:    col.Name,
			GoType:      goType,
			SchemaType:  goTypeToSchemaType(goType),
			Description: col.Comment,
			Required:    !col.IsNullable || col.IsPrimary,
		})
	}
	return out
}

func dbPrimaryKeyFields(schema *dbschema.TableSchema) []*Field {
	var out []*Field
	for _, field := range dbMessageFields(schema) {
		for _, col := range schema.Columns {
			if col.Name == field.JSONName && col.IsPrimary {
				out = append(out, field)
				break
			}
		}
	}
	return out
}

func dbNonPrimaryFields(fields []*Field) []*Field {
	out := make([]*Field, 0, len(fields))
	for _, field := range fields {
		if field.JSONName == "id" || strings.HasSuffix(strings.ToLower(field.Name), "id") && field.Required {
			continue
		}
		out = append(out, field)
	}
	return out
}

func filterCreateFields(fields []*Field) []*Field {
	out := make([]*Field, 0, len(fields))
	for _, field := range fields {
		if field.JSONName == "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func fallbackPrimaryFields(fields []*Field) []*Field {
	if len(fields) > 0 {
		return fields
	}
	return []*Field{{Name: "ID", JSONName: "id", GoType: "string", SchemaType: "string", Required: true}}
}

func cloneFields(fields []*Field) []*Field {
	out := make([]*Field, 0, len(fields))
	for _, field := range fields {
		copied := *field
		out = append(out, &copied)
	}
	return out
}

func singularModelName(tableName string) string {
	return dbschema.SnakeToCamel(singularizeTable(tableName))
}

func singularizeTable(s string) string {
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasSuffix(lower, "ies") && len(s) > 3:
		return s[:len(s)-3] + "y"
	case strings.HasSuffix(lower, "ses"),
		strings.HasSuffix(lower, "xes"),
		strings.HasSuffix(lower, "zes"),
		strings.HasSuffix(lower, "ches"),
		strings.HasSuffix(lower, "shes"):
		return s[:len(s)-2]
	case strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss"):
		return s[:len(s)-1]
	default:
		return s
	}
}

func isGormAutoField(name string) bool {
	switch strings.ToLower(name) {
	case "created_at", "updated_at", "deleted_at":
		return true
	default:
		return false
	}
}

func dbColumnGoType(col dbschema.ColumnInfo) string {
	t := strings.ToLower(strings.TrimSpace(col.DBType))
	if idx := strings.Index(t, "("); idx >= 0 {
		t = t[:idx]
	}

	var goType string
	switch {
	case t == "tinyint" || t == "bool" || t == "boolean":
		goType = "bool"
	case t == "smallint" || t == "int2":
		goType = "int16"
	case t == "mediumint" || t == "int" || t == "integer" || t == "int4":
		goType = "int"
	case t == "bigint" || t == "int8" || t == "serial" || t == "bigserial":
		goType = "int64"
	case strings.Contains(t, "unsigned"):
		goType = "uint"
	case t == "float" || t == "real" || t == "float4":
		goType = "float32"
	case t == "double" || t == "decimal" || t == "numeric" || t == "float8":
		goType = "float64"
	case t == "char" || t == "varchar" || t == "tinytext" || t == "text" ||
		t == "mediumtext" || t == "longtext" || t == "nvarchar" || t == "nchar" ||
		t == "character varying" || t == "character":
		goType = "string"
	case t == "date" || t == "datetime" || t == "timestamp" || t == "timestamptz" ||
		t == "time" || t == "timetz":
		goType = "time.Time"
	case t == "json" || t == "jsonb":
		goType = "string"
	case t == "blob" || t == "bytea" || t == "binary" || t == "varbinary":
		goType = "[]byte"
	default:
		goType = "string"
	}

	if col.IsNullable && !col.IsPrimary && goType != "[]byte" && !strings.HasPrefix(goType, "[]") {
		return "*" + goType
	}
	return goType
}

func dbColumnGormTag(col dbschema.ColumnInfo) string {
	parts := []string{"column:" + col.Name}
	if col.IsPrimary {
		parts = append(parts, "primaryKey")
	}
	if col.IsAutoIncr {
		parts = append(parts, "autoIncrement")
	}
	if !col.IsNullable && !col.IsPrimary {
		parts = append(parts, "not null")
	}
	if col.IsUnique && !col.IsPrimary {
		parts = append(parts, "uniqueIndex")
	}
	if dbType := strings.TrimSpace(col.DBType); dbType != "" {
		parts = append(parts, "type:"+dbType)
	}
	return strings.Join(parts, ";")
}
