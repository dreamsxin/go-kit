package generator_test

import (
	"io/fs"
	"os"
	"testing"
)

// testTemplateFS 是测试中使用的模板文件系统。
// 模板位于 cmd/microgen/templates/，在 generator 包的视角下是父目录。
// 用 os.DirFS("..") 包裹后可以用 "templates/*.tmpl" 路径访问。
var testTemplateFS fs.FS

func TestMain(m *testing.M) {
	// 从磁盘读取模板（os.DirFS 实现了 fs.FS，与 generator.Options.TemplateFS 类型匹配）
	testTemplateFS = os.DirFS("..")
	os.Exit(m.Run())
}
