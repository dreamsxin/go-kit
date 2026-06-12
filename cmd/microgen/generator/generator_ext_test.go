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
	mustContain(t, skillPath, "type SkillMetadata struct")
	mustContain(t, skillPath, `SchemaVersion: "microgen.skill.v1"`)
	mustContain(t, skillPath, `Source:        "microgen-ir"`)
	mustContain(t, skillPath, `"UserService"`)
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

func TestGenerateFull_Interaction(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:       outDir,
		ImportPath:      "example.com/interactiontest",
		WithInteraction: true,
		WithSkill:       true,
		WithConfig:      false,
		WithDocs:        false,
		DBDriver:        "sqlite",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	interactionPath := filepath.Join(outDir, "cmd", "generated_interaction.go")
	mustExist(t, interactionPath)
	mustContain(t, interactionPath, "type CreateUserTool struct")
	mustContain(t, interactionPath, "func (t CreateUserTool) Name() string")
	mustContain(t, interactionPath, "func (t CreateUserTool) Call(")
	mustContain(t, interactionPath, "func (t CreateUserTool) Descriptor() interaction.ToolDescriptor")
	mustContain(t, interactionPath, "func initInteractionRuntime(svc *generatedServices) *interaction.Runtime")
	mustContain(t, interactionPath, `Name:        "CreateUser"`)
	mustContain(t, interactionPath, `"username": map[string]interface{}{
					"type": "string"`)
	mustContain(t, interactionPath, `"email": map[string]interface{}{
					"type": "string"`)

	guidePath := filepath.Join(outDir, ".ai", "PROJECT_GUIDE.md")
	mustExist(t, guidePath)
	mustContain(t, guidePath, "POST /mcp")
	mustContain(t, guidePath, "cmd/generated_interaction.go")
	mustContain(t, guidePath, "Validation Commands")

	readmePath := filepath.Join(outDir, "README.md")
	mustNotExist(t, readmePath) // WithDocs=false
}

func TestGenerateFull_InteractionWithoutSkill(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:       outDir,
		ImportPath:      "example.com/interaction_noskill",
		WithInteraction: true,
		WithSkill:       false,
		WithConfig:      false,
		WithDocs:        false,
		DBDriver:        "sqlite",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	interactionPath := filepath.Join(outDir, "cmd", "generated_interaction.go")
	mustExist(t, interactionPath)
	guidePath := filepath.Join(outDir, ".ai", "PROJECT_GUIDE.md")
	mustExist(t, guidePath)
}

func TestGenerateFull_NoInteraction(t *testing.T) {
	outDir := newTmpDir(t)
	project := parseIDLProject(t, "basic.go")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:       outDir,
		ImportPath:      "example.com/nointeraction",
		WithInteraction: false,
		WithSkill:       true,
		WithConfig:      false,
		WithDocs:        false,
		DBDriver:        "sqlite",
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	interactionPath := filepath.Join(outDir, "cmd", "generated_interaction.go")
	mustNotExist(t, interactionPath)
	guidePath := filepath.Join(outDir, ".ai", "PROJECT_GUIDE.md")
	mustNotExist(t, guidePath)
}
