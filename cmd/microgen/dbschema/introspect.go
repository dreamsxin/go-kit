// Package dbschema 连接数据库，内省表结构，并将其转换为 parser.ParseResult，
// 供 generator 直接生成完整的 RESTful 微服务代码。
//
// 支持驱动：mysql / postgres / sqlite / sqlserver
package dbschema

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// TableSchema 单张表的列信息
type TableSchema struct {
	TableName string
	Columns   []ColumnInfo
}

// ColumnInfo 单列元数据
type ColumnInfo struct {
	Name       string // 列名
	DBType     string // 数据库原始类型（varchar(64)、int、bigint unsigned …）
	IsNullable bool   // 是否允许 NULL
	IsPrimary  bool   // 是否主键
	IsAutoIncr bool   // 是否自增
	IsUnique   bool   // 是否唯一索引
	Comment    string // 列注释（MySQL/Postgres 支持）
	Default    string // 默认值（可为空）
}

// Introspector 数据库内省器接口
type Introspector interface {
	// Tables 返回数据库中所有（或指定）表的 schema
	Tables(db *sql.DB, dbName string, tables []string) ([]*TableSchema, error)
}

// NewIntrospector 根据驱动名返回对应的内省器
func NewIntrospector(driver string) (Introspector, error) {
	switch strings.ToLower(driver) {
	case "mysql":
		return &mysqlIntrospector{}, nil
	case "postgres", "postgresql":
		return &postgresIntrospector{}, nil
	case "sqlite", "sqlite3":
		return &sqliteIntrospector{}, nil
	case "sqlserver", "mssql":
		return &sqlserverIntrospector{}, nil
	default:
		return nil, fmt.Errorf("unsupported driver for schema introspection: %q", driver)
	}
}

// ─────────────────────────── MySQL ───────────────────────────

type mysqlIntrospector struct{}

func (m *mysqlIntrospector) Tables(db *sql.DB, dbName string, tables []string) ([]*TableSchema, error) {
	// 查询所有表名
	tableNames, err := m.listTables(db, dbName, tables)
	if err != nil {
		return nil, err
	}

	var result []*TableSchema
	for _, tbl := range tableNames {
		cols, err := m.columns(db, dbName, tbl)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", tbl, err)
		}
		result = append(result, &TableSchema{TableName: tbl, Columns: cols})
	}
	return result, nil
}

func (m *mysqlIntrospector) listTables(db *sql.DB, dbName string, filter []string) ([]string, error) {
	rows, err := db.Query(
		`SELECT TABLE_NAME FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		 ORDER BY TABLE_NAME`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filterSet := toSet(filter)
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if len(filterSet) == 0 || filterSet[name] {
			names = append(names, name)
		}
	}
	return names, rows.Err()
}

func (m *mysqlIntrospector) columns(db *sql.DB, dbName, table string) ([]ColumnInfo, error) {
	rows, err := db.Query(`
		SELECT
			c.COLUMN_NAME,
			c.COLUMN_TYPE,
			c.IS_NULLABLE,
			c.COLUMN_KEY,
			c.EXTRA,
			COALESCE(c.COLUMN_COMMENT, ''),
			COALESCE(c.COLUMN_DEFAULT, '')
		FROM information_schema.COLUMNS c
		WHERE c.TABLE_SCHEMA = ? AND c.TABLE_NAME = ?
		ORDER BY c.ORDINAL_POSITION`, dbName, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var (
			name, colType, nullable, colKey, extra, comment, def string
		)
		if err := rows.Scan(&name, &colType, &nullable, &colKey, &extra, &comment, &def); err != nil {
			return nil, err
		}
		cols = append(cols, ColumnInfo{
			Name:       name,
			DBType:     colType,
			IsNullable: strings.EqualFold(nullable, "YES"),
			IsPrimary:  strings.EqualFold(colKey, "PRI"),
			IsAutoIncr: strings.Contains(strings.ToLower(extra), "auto_increment"),
			IsUnique:   strings.EqualFold(colKey, "UNI"),
			Comment:    comment,
			Default:    def,
		})
	}
	return cols, rows.Err()
}

// ─────────────────────────── PostgreSQL ───────────────────────────

type postgresIntrospector struct{}

func (p *postgresIntrospector) Tables(db *sql.DB, dbName string, tables []string) ([]*TableSchema, error) {
	tableNames, err := p.listTables(db, tables)
	if err != nil {
		return nil, err
	}
	var result []*TableSchema
	for _, tbl := range tableNames {
		cols, err := p.columns(db, tbl)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", tbl, err)
		}
		result = append(result, &TableSchema{TableName: tbl, Columns: cols})
	}
	return result, nil
}

func (p *postgresIntrospector) listTables(db *sql.DB, filter []string) ([]string, error) {
	rows, err := db.Query(`
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		ORDER BY tablename`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filterSet := toSet(filter)
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if len(filterSet) == 0 || filterSet[name] {
			names = append(names, name)
		}
	}
	return names, rows.Err()
}

func (p *postgresIntrospector) columns(db *sql.DB, table string) ([]ColumnInfo, error) {
	rows, err := db.Query(`
		SELECT
			a.attname AS column_name,
			pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
			NOT a.attnotnull AS is_nullable,
			COALESCE(
				(SELECT true FROM pg_index i
				 JOIN pg_attribute ia ON ia.attrelid = i.indrelid AND ia.attnum = ANY(i.indkey)
				 WHERE i.indrelid = a.attrelid AND ia.attnum = a.attnum AND i.indisprimary
				 LIMIT 1), false) AS is_primary,
			COALESCE(pg_get_expr(d.adbin, d.adrelid), '') AS column_default,
			COALESCE(col_description(a.attrelid, a.attnum), '') AS comment
		FROM pg_attribute a
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var (
			name, dataType, def, comment string
			isNullable, isPrimary        bool
		)
		if err := rows.Scan(&name, &dataType, &isNullable, &isPrimary, &def, &comment); err != nil {
			return nil, err
		}
		isAutoIncr := strings.Contains(def, "nextval(") || strings.HasPrefix(dataType, "serial")
		cols = append(cols, ColumnInfo{
			Name:       name,
			DBType:     dataType,
			IsNullable: isNullable,
			IsPrimary:  isPrimary,
			IsAutoIncr: isAutoIncr,
			Comment:    comment,
			Default:    def,
		})
	}
	return cols, rows.Err()
}

// ─────────────────────────── SQLite ───────────────────────────

type sqliteIntrospector struct{}

func (s *sqliteIntrospector) Tables(db *sql.DB, _ string, tables []string) ([]*TableSchema, error) {
	tableNames, err := s.listTables(db, tables)
	if err != nil {
		return nil, err
	}
	var result []*TableSchema
	for _, tbl := range tableNames {
		cols, err := s.columns(db, tbl)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", tbl, err)
		}
		result = append(result, &TableSchema{TableName: tbl, Columns: cols})
	}
	return result, nil
}

func (s *sqliteIntrospector) listTables(db *sql.DB, filter []string) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filterSet := toSet(filter)
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if len(filterSet) == 0 || filterSet[name] {
			names = append(names, name)
		}
	}
	return names, rows.Err()
}

func (s *sqliteIntrospector) columns(db *sql.DB, table string) ([]ColumnInfo, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var (
			cid                   int
			name, colType         string
			notNull, isPrimaryKey int
			def                   sql.NullString
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &def, &isPrimaryKey); err != nil {
			return nil, err
		}
		cols = append(cols, ColumnInfo{
			Name:       name,
			DBType:     colType,
			IsNullable: notNull == 0,
			IsPrimary:  isPrimaryKey == 1,
			IsAutoIncr: isPrimaryKey == 1 && strings.Contains(strings.ToUpper(colType), "INT"),
			Default:    def.String,
		})
	}
	return cols, rows.Err()
}

// ─────────────────────────── SQL Server ───────────────────────────

type sqlserverIntrospector struct{}

func (ss *sqlserverIntrospector) Tables(db *sql.DB, dbName string, tables []string) ([]*TableSchema, error) {
	tableNames, err := ss.listTables(db, tables)
	if err != nil {
		return nil, err
	}
	var result []*TableSchema
	for _, tbl := range tableNames {
		cols, err := ss.columns(db, tbl)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", tbl, err)
		}
		result = append(result, &TableSchema{TableName: tbl, Columns: cols})
	}
	return result, nil
}

func (ss *sqlserverIntrospector) listTables(db *sql.DB, filter []string) ([]string, error) {
	rows, err := db.Query(`SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filterSet := toSet(filter)
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if len(filterSet) == 0 || filterSet[name] {
			names = append(names, name)
		}
	}
	return names, rows.Err()
}

func (ss *sqlserverIntrospector) columns(db *sql.DB, table string) ([]ColumnInfo, error) {
	rows, err := db.Query(`
		SELECT
			c.COLUMN_NAME,
			c.DATA_TYPE,
			c.IS_NULLABLE,
			COALESCE(c.COLUMN_DEFAULT, '')
		FROM INFORMATION_SCHEMA.COLUMNS c
		WHERE c.TABLE_NAME = @p1
		ORDER BY c.ORDINAL_POSITION`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 查询主键
	pkRows, err := db.Query(`
		SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
		WHERE TABLE_NAME = @p1 AND CONSTRAINT_NAME LIKE 'PK_%'`, table)
	if err != nil {
		return nil, err
	}
	defer pkRows.Close()
	pkSet := map[string]bool{}
	for pkRows.Next() {
		var col string
		if err := pkRows.Scan(&col); err != nil {
			return nil, err
		}
		pkSet[col] = true
	}

	var cols []ColumnInfo
	for rows.Next() {
		var name, dataType, nullable, def string
		if err := rows.Scan(&name, &dataType, &nullable, &def); err != nil {
			return nil, err
		}
		cols = append(cols, ColumnInfo{
			Name:       name,
			DBType:     dataType,
			IsNullable: strings.EqualFold(nullable, "YES"),
			IsPrimary:  pkSet[name],
			IsAutoIncr: pkSet[name], // SQL Server identity 列通常是 PK
			Default:    def,
		})
	}
	return cols, rows.Err()
}

// ─────────────────────────── 辅助 ───────────────────────────

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, v := range items {
		if v != "" {
			s[v] = true
		}
	}
	return s
}

// ─────────────────────────── Schema → ParseResult ───────────────────────────

// ToParseResult 将数据库表 schema 转换为 parser.ParseResult，
// 每张表生成一个 Model 和对应的 CRUD Service。
func ToParseResult(schemas []*TableSchema, serviceName, pkgName string) *parser.ParseResult {
	if pkgName == "" {
		pkgName = strings.ToLower(serviceName)
	}

	result := &parser.ParseResult{
		PackageName: pkgName,
	}

	for _, schema := range schemas {
		model := tableToModel(schema)
		result.Models = append(result.Models, model)
	}

	// 为所有表生成一个聚合 Service（包含每张表的 CRUD 方法）
	svc := buildService(schemas, serviceName)
	result.Services = append(result.Services, svc)

	return result
}

// tableToModel 将单张表转换为 parser.Model
func tableToModel(schema *TableSchema) *parser.Model {
	// 表名通常是复数（users, orders），模型名应为单数（User, Order）
	singularName := singularize(schema.TableName)
	model := &parser.Model{
		Name:        snakeToCamel(singularName),
		TableName:   schema.TableName,
		HasGormTags: true,
	}

	// GORM 模板会自动追加 CreatedAt/UpdatedAt/DeletedAt，跳过这些列避免重复
	gormAutoFields := map[string]bool{
		"created_at": true,
		"updated_at": true,
		"deleted_at": true,
	}

	for _, col := range schema.Columns {
		if gormAutoFields[strings.ToLower(col.Name)] {
			continue
		}
		field := columnToField(col)
		model.Fields = append(model.Fields, field)
	}
	return model
}

// columnToField 将列信息转换为 parser.ModelField
func columnToField(col ColumnInfo) parser.ModelField {
	// 主键字段不应为 nullable
	nullable := col.IsNullable && !col.IsPrimary
	goType := dbTypeToGoType(col.DBType, nullable)
	jsonTag := col.Name
	gormParts := []string{"column:" + col.Name}
	if col.IsPrimary {
		gormParts = append(gormParts, "primaryKey")
	}
	if col.IsAutoIncr {
		gormParts = append(gormParts, "autoIncrement")
	}
	if !col.IsNullable && !col.IsPrimary {
		gormParts = append(gormParts, "not null")
	}
	if col.IsUnique && !col.IsPrimary {
		gormParts = append(gormParts, "uniqueIndex")
	}
	if col.DBType != "" {
		gormParts = append(gormParts, "type:"+normalizeDBType(col.DBType))
	}

	return parser.ModelField{
		Name:       snakeToCamel(col.Name),
		Type:       goType,
		JSONTag:    jsonTag,
		GormTag:    strings.Join(gormParts, ";"),
		Comment:    col.Comment,
		IsPrimary:  col.IsPrimary,
		IsAutoIncr: col.IsAutoIncr,
		IsNotNull:  !col.IsNullable,
		IsUnique:   col.IsUnique,
		SwagType:   goTypeToSwagType(goType),
	}
}

// buildService 为所有表构建一个聚合 Service，每张表生成标准 CRUD 方法
func buildService(schemas []*TableSchema, serviceName string) *parser.Service {
	svc := &parser.Service{
		ServiceName: serviceName,
		PackageName: strings.ToLower(serviceName),
		Title:       serviceName + " API",
		Description: "Auto-generated RESTful API from database schema",
	}

	for _, schema := range schemas {
		modelName := snakeToCamel(schema.TableName)
		methods := crudMethods(modelName)
		svc.Methods = append(svc.Methods, methods...)
	}
	return svc
}

// crudMethods 为单个 model 生成标准 CRUD 方法定义
func crudMethods(modelName string) []parser.Method {
	lower := strings.ToLower(modelName)
	return []parser.Method{
		{
			Name:       "Create" + modelName,
			Input:      "Create" + modelName + "Request",
			Output:     "Create" + modelName + "Response",
			HTTPMethod: "post",
			Route:      "/" + lower,
			Doc:        "创建 " + modelName,
			Summary:    "创建 " + modelName,
			Tags:       modelName,
		},
		{
			Name:       "Get" + modelName,
			Input:      "Get" + modelName + "Request",
			Output:     "Get" + modelName + "Response",
			HTTPMethod: "get",
			Route:      "/" + lower + "/{id}",
			Doc:        "获取 " + modelName + " 详情",
			Summary:    "获取 " + modelName + " 详情",
			Tags:       modelName,
		},
		{
			Name:       "Update" + modelName,
			Input:      "Update" + modelName + "Request",
			Output:     "Update" + modelName + "Response",
			HTTPMethod: "put",
			Route:      "/" + lower + "/{id}",
			Doc:        "更新 " + modelName,
			Summary:    "更新 " + modelName,
			Tags:       modelName,
		},
		{
			Name:       "Delete" + modelName,
			Input:      "Delete" + modelName + "Request",
			Output:     "Delete" + modelName + "Response",
			HTTPMethod: "delete",
			Route:      "/" + lower + "/{id}",
			Doc:        "删除 " + modelName,
			Summary:    "删除 " + modelName,
			Tags:       modelName,
		},
		{
			Name:       "List" + modelName + "s",
			Input:      "List" + modelName + "sRequest",
			Output:     "List" + modelName + "sResponse",
			HTTPMethod: "get",
			Route:      "/" + lower + "s",
			Doc:        "分页查询 " + modelName + " 列表",
			Summary:    "分页查询 " + modelName + " 列表",
			Tags:       modelName,
		},
	}
}

// ─────────────────────────── 类型映射 ───────────────────────────

// dbTypeToGoType 将数据库列类型映射为 Go 类型
func dbTypeToGoType(dbType string, nullable bool) string {
	t := strings.ToLower(dbType)
	// 去掉括号内的精度/长度，如 varchar(64) → varchar
	if idx := strings.Index(t, "("); idx != -1 {
		t = t[:idx]
	}
	t = strings.TrimSpace(t)

	var goType string
	switch {
	case t == "tinyint" || t == "bool" || t == "boolean":
		goType = "bool"
	case t == "smallint" || t == "int2":
		goType = "int16"
	case t == "mediumint" || t == "int" || t == "integer" || t == "int4":
		goType = "int"
	case t == "bigint" || t == "int8":
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
		goType = "string" // 简化处理，实际可用 json.RawMessage
	case t == "blob" || t == "bytea" || t == "binary" || t == "varbinary":
		goType = "[]byte"
	case t == "serial" || t == "bigserial":
		goType = "int64"
	default:
		goType = "string"
	}

	if nullable && goType != "[]byte" && !strings.HasPrefix(goType, "[]") {
		return "*" + goType
	}
	return goType
}

// normalizeDBType 规范化 DB 类型字符串，用于 gorm tag
func normalizeDBType(dbType string) string {
	// 去掉多余空格，保留括号内容
	return strings.TrimSpace(dbType)
}

// goTypeToSwagType 复用 parser 包的逻辑（避免循环依赖，直接内联）
func goTypeToSwagType(goType string) string {
	t := strings.TrimPrefix(goType, "*")
	switch t {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "bool":
		return "boolean"
	case "string":
		return "string"
	default:
		if strings.HasPrefix(t, "[]") {
			return "array"
		}
		return "object"
	}
}

// ─────────────────────────── 命名转换 ───────────────────────────

// SnakeToCamel 将 snake_case 转换为 CamelCase（首字母大写，导出供外部使用）
func SnakeToCamel(s string) string {
	return snakeToCamel(s)
}

// snakeToCamel 将 snake_case 转换为 CamelCase（首字母大写）
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	var sb strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		sb.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return sb.String()
}

// singularize 将英文复数名词转换为单数（简单规则，覆盖常见情况）
func singularize(s string) string {
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	// 不规则复数
	irregulars := map[string]string{
		"people":   "person",
		"men":      "man",
		"women":    "woman",
		"children": "child",
		"mice":     "mouse",
		"geese":    "goose",
		"teeth":    "tooth",
		"feet":     "foot",
		"oxen":     "ox",
	}
	if singular, ok := irregulars[lower]; ok {
		return singular
	}
	// -ies → -y (e.g. categories → category)
	if strings.HasSuffix(lower, "ies") && len(lower) > 3 {
		return s[:len(s)-3] + "y"
	}
	// -ses / -xes / -zes / -ches / -shes → remove -es
	if strings.HasSuffix(lower, "ses") || strings.HasSuffix(lower, "xes") ||
		strings.HasSuffix(lower, "zes") || strings.HasSuffix(lower, "ches") ||
		strings.HasSuffix(lower, "shes") {
		return s[:len(s)-2]
	}
	// -s → remove (but not -ss)
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") {
		return s[:len(s)-1]
	}
	return s
}
