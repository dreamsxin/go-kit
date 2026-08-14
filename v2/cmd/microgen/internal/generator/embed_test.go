package generator_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// testTemplateFS resolves templates from the microgen module root.
var testTemplateFS fs.FS

func TestMain(m *testing.M) {
	testTemplateFS = os.DirFS(filepath.Join("..", ".."))
	os.Exit(m.Run())
}
