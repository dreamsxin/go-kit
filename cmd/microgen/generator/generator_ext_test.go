package generator_test

import (
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
)

func TestGenerateFull_Skill(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/skilltest",
		WithSkill:  true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	skillPath := filepath.Join(outDir, "skill", "skill.go")
	mustExist(t, skillPath)
	mustContain(t, skillPath, "func Handler(w http.ResponseWriter, r *http.Request)")
	mustContain(t, skillPath, "getOpenAITools()")
	mustContain(t, skillPath, "getMCPTools()")
}

func TestGenerateFull_SDK(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/sdktest",
		WithGRPC:   true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	sdkPath := filepath.Join(outDir, "sdk", "userservicesdk", "client.go")
	mustExist(t, sdkPath)
	mustContain(t, sdkPath, "type Client interface")
	mustContain(t, sdkPath, "func New(baseURL string, opts ...Option) Client")
	mustContain(t, sdkPath, "func NewGRPC(conn *grpc.ClientConn) Client")
}

func TestGenerateFull_ModelsAndHooks(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/hooktest",
		WithModel:  true,
		DBDriver:   "sqlite",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	modelPath := filepath.Join(outDir, "model", "user.go")
	mustExist(t, modelPath)
	mustContain(t, modelPath, "func (m *User) BeforeCreate(tx *gorm.DB) (err error)")
	mustContain(t, modelPath, "func (m *User) BeforeUpdate(tx *gorm.DB) (err error)")
}
