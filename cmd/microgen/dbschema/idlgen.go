package dbschema

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// WriteIDL 将数据库 schema 写成 idl.go 文件（包含 GORM model + DTO + Service 接口），
// 输出到 outDir/idl.go，供后续 parser.ParseFull 解析再生成完整项目。
// 返回生成的 idl.go 绝对路径。
func WriteIDL(schemas []*TableSchema, pkgName, outDir string) (string, error) {
	if pkgName == "" {
		pkgName = "idl"
	}

	models := make([]*parser.Model, 0, len(schemas))
	for _, s := range schemas {
		models = append(models, tableToModel(s))
	}

	var sb strings.Builder

	sb.WriteString("package " + pkgName + "\n\n")

	// 检查是否需要 time 包
	needTime := false
	for _, m := range models {
		for _, f := range m.Fields {
			if strings.Contains(f.Type, "time.Time") {
				needTime = true
				break
			}
		}
	}
	imports := []string{`"context"`}
	if needTime {
		imports = append(imports, `"time"`)
	}
	sb.WriteString("import (\n")
	for _, imp := range imports {
		sb.WriteString("\t" + imp + "\n")
	}
	sb.WriteString(")\n\n")

	// ── GORM Models ──
	sb.WriteString("// ─────────────────────────── GORM Models ───────────────────────────\n\n")
	for i, m := range models {
		schema := schemas[i]
		sb.WriteString(fmt.Sprintf("// %s 对应数据库表 %s\n", m.Name, schema.TableName))
		sb.WriteString(fmt.Sprintf("type %s struct {\n", m.Name))
		for _, f := range m.Fields {
			tag := buildTag(f.JSONTag, f.GormTag)
			comment := ""
			if f.Comment != "" {
				comment = " // " + f.Comment
			}
			sb.WriteString(fmt.Sprintf("\t%s %s%s%s\n", f.Name, f.Type, tag, comment))
		}
		sb.WriteString("}\n\n")
	}

	// ── DTOs ──
	sb.WriteString("// ─────────────────────────── DTOs ───────────────────────────\n\n")
	for _, m := range models {
		writeDTOs(&sb, m)
	}

	// ── Service 接口 ──
	sb.WriteString("// ─────────────────────────── Service 接口 ───────────────────────────\n\n")
	svcName := toServiceName(pkgName)
	sb.WriteString(fmt.Sprintf("// %s 自动生成的 RESTful 服务接口\n", svcName))
	sb.WriteString(fmt.Sprintf("type %s interface {\n", svcName))
	for _, m := range models {
		writeServiceMethods(&sb, m.Name)
	}
	sb.WriteString("}\n")

	// 写文件
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	idlPath := filepath.Join(outDir, "idl.go")
	if err := os.WriteFile(idlPath, []byte(sb.String()), 0644); err != nil {
		return "", err
	}
	return idlPath, nil
}

// buildTag 构建 struct tag 字符串
func buildTag(jsonTag, gormTag string) string {
	if jsonTag == "" && gormTag == "" {
		return ""
	}
	parts := []string{}
	if jsonTag != "" {
		parts = append(parts, fmt.Sprintf(`json:"%s"`, jsonTag))
	}
	if gormTag != "" {
		parts = append(parts, fmt.Sprintf(`gorm:"%s"`, gormTag))
	}
	return " `" + strings.Join(parts, " ") + "`"
}

// writeDTOs 为单个 model 写出 CRUD 所需的 DTO 结构体
func writeDTOs(sb *strings.Builder, m *parser.Model) {
	name := m.Name

	// 对外暴露的实体（不含 gorm 内部字段）
	sb.WriteString(fmt.Sprintf("// %sItem 对外暴露的 %s 实体\n", name, name))
	sb.WriteString(fmt.Sprintf("type %sItem struct {\n", name))
	for _, f := range m.Fields {
		sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s\"`\n", f.Name, f.Type, f.JSONTag))
	}
	sb.WriteString("}\n\n")

	// Create Request（跳过主键/自增字段）
	sb.WriteString(fmt.Sprintf("// Create%sRequest 创建请求\n", name))
	sb.WriteString(fmt.Sprintf("type Create%sRequest struct {\n", name))
	for _, f := range m.Fields {
		if f.IsPrimary || f.IsAutoIncr {
			continue
		}
		sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s\"`\n", f.Name, f.Type, f.JSONTag))
	}
	sb.WriteString("}\n\n")

	sb.WriteString(fmt.Sprintf("// Create%sResponse 创建响应\n", name))
	sb.WriteString(fmt.Sprintf("type Create%sResponse struct {\n", name))
	sb.WriteString(fmt.Sprintf("\tData  *%sItem `json:\"data,omitempty\"`\n", name))
	sb.WriteString("\tError string    `json:\"error,omitempty\"`\n")
	sb.WriteString("}\n\n")

	// Get
	sb.WriteString(fmt.Sprintf("// Get%sRequest 获取请求\n", name))
	sb.WriteString(fmt.Sprintf("type Get%sRequest struct {\n", name))
	sb.WriteString("\tID uint `json:\"id\"`\n")
	sb.WriteString("}\n\n")

	sb.WriteString(fmt.Sprintf("// Get%sResponse 获取响应\n", name))
	sb.WriteString(fmt.Sprintf("type Get%sResponse struct {\n", name))
	sb.WriteString(fmt.Sprintf("\tData  *%sItem `json:\"data,omitempty\"`\n", name))
	sb.WriteString("\tError string    `json:\"error,omitempty\"`\n")
	sb.WriteString("}\n\n")

	// Update（非主键字段用指针，支持 omitempty 语义）
	sb.WriteString(fmt.Sprintf("// Update%sRequest 更新请求\n", name))
	sb.WriteString(fmt.Sprintf("type Update%sRequest struct {\n", name))
	sb.WriteString("\tID uint `json:\"id\"`\n")
	for _, f := range m.Fields {
		if f.IsPrimary || f.IsAutoIncr {
			continue
		}
		goType := f.Type
		if !strings.HasPrefix(goType, "*") {
			goType = "*" + goType
		}
		sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s,omitempty\"`\n", f.Name, goType, f.JSONTag))
	}
	sb.WriteString("}\n\n")

	sb.WriteString(fmt.Sprintf("// Update%sResponse 更新响应\n", name))
	sb.WriteString(fmt.Sprintf("type Update%sResponse struct {\n", name))
	sb.WriteString(fmt.Sprintf("\tData  *%sItem `json:\"data,omitempty\"`\n", name))
	sb.WriteString("\tError string    `json:\"error,omitempty\"`\n")
	sb.WriteString("}\n\n")

	// Delete
	sb.WriteString(fmt.Sprintf("// Delete%sRequest 删除请求\n", name))
	sb.WriteString(fmt.Sprintf("type Delete%sRequest struct {\n", name))
	sb.WriteString("\tID uint `json:\"id\"`\n")
	sb.WriteString("}\n\n")

	sb.WriteString(fmt.Sprintf("// Delete%sResponse 删除响应\n", name))
	sb.WriteString(fmt.Sprintf("type Delete%sResponse struct {\n", name))
	sb.WriteString("\tSuccess bool   `json:\"success\"`\n")
	sb.WriteString("\tError   string `json:\"error,omitempty\"`\n")
	sb.WriteString("}\n\n")

	// List
	sb.WriteString(fmt.Sprintf("// List%ssRequest 列表查询请求\n", name))
	sb.WriteString(fmt.Sprintf("type List%ssRequest struct {\n", name))
	sb.WriteString("\tPage     int    `json:\"page\"`\n")
	sb.WriteString("\tPageSize int    `json:\"page_size\"`\n")
	sb.WriteString("\tKeyword  string `json:\"keyword,omitempty\"`\n")
	sb.WriteString("}\n\n")

	sb.WriteString(fmt.Sprintf("// List%ssResponse 列表查询响应\n", name))
	sb.WriteString(fmt.Sprintf("type List%ssResponse struct {\n", name))
	sb.WriteString(fmt.Sprintf("\tData     []%sItem `json:\"data\"`\n", name))
	sb.WriteString("\tTotal    int64      `json:\"total\"`\n")
	sb.WriteString("\tPage     int        `json:\"page\"`\n")
	sb.WriteString("\tPageSize int        `json:\"page_size\"`\n")
	sb.WriteString("\tError    string     `json:\"error,omitempty\"`\n")
	sb.WriteString("}\n\n")
}

// writeServiceMethods 为单个 model 写出 Service 接口方法
func writeServiceMethods(sb *strings.Builder, modelName string) {
	sb.WriteString(fmt.Sprintf("\t// Create%s 创建\n", modelName))
	sb.WriteString(fmt.Sprintf("\tCreate%s(ctx context.Context, req Create%sRequest) (Create%sResponse, error)\n\n", modelName, modelName, modelName))

	sb.WriteString(fmt.Sprintf("\t// Get%s 获取详情\n", modelName))
	sb.WriteString(fmt.Sprintf("\tGet%s(ctx context.Context, req Get%sRequest) (Get%sResponse, error)\n\n", modelName, modelName, modelName))

	sb.WriteString(fmt.Sprintf("\t// Update%s 更新\n", modelName))
	sb.WriteString(fmt.Sprintf("\tUpdate%s(ctx context.Context, req Update%sRequest) (Update%sResponse, error)\n\n", modelName, modelName, modelName))

	sb.WriteString(fmt.Sprintf("\t// Delete%s 删除\n", modelName))
	sb.WriteString(fmt.Sprintf("\tDelete%s(ctx context.Context, req Delete%sRequest) (Delete%sResponse, error)\n\n", modelName, modelName, modelName))

	sb.WriteString(fmt.Sprintf("\t// List%ss 分页查询\n", modelName))
	sb.WriteString(fmt.Sprintf("\tList%ss(ctx context.Context, req List%ssRequest) (List%ssResponse, error)\n\n", modelName, modelName, modelName))
}

// toServiceName 将 pkgName 转换为 ServiceName（首字母大写 + Service 后缀）
func toServiceName(pkgName string) string {
	if pkgName == "" {
		return "Service"
	}
	name := snakeToCamel(pkgName)
	if !strings.HasSuffix(name, "Service") {
		name += "Service"
	}
	return name
}
