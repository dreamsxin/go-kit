package generator_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/dbschema"
	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

func TestGenerateProject_FromDBIR_GeneratesArtifactsFromExplicitIR(t *testing.T) {
	outDir := newTmpDir(t)
	schemas := []*dbschema.TableSchema{
		{
			TableName: "users",
			Columns: []dbschema.ColumnInfo{
				{Name: "id", DBType: "bigint", IsPrimary: true, IsAutoIncr: true},
				{Name: "username", DBType: "varchar(64)", IsNullable: false},
				{Name: "email", DBType: "varchar(128)", IsNullable: false},
			},
		},
	}
	project := ir.FromTableSchemas(schemas, "UserAdminService", "useradmin")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/useradmin",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   true,
		WithSkill:  true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "service", "useradminservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "useradminservice", "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "transport", "useradminservice", "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "model", "generated_user.go"))
	mustExist(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
	mustExist(t, filepath.Join(outDir, "cmd", "main.go"))
	mustExist(t, filepath.Join(outDir, "README.md"))
	mustExist(t, filepath.Join(outDir, "skill", "skill.go"))

	mustContain(t, filepath.Join(outDir, "service", "useradminservice", "service.go"), "CreateUser")
	mustContain(t, filepath.Join(outDir, "transport", "useradminservice", "transport_http.go"), "ListUsers")
	mustContain(t, filepath.Join(outDir, "README.md"), "UserAdminService")
	mustContain(t, filepath.Join(outDir, "skill", "skill.go"), "DeleteUser")
}

func TestGenerateProject_FromDBIR_GeneratesModelArtifactsWithoutCompatParseResult(t *testing.T) {
	outDir := newTmpDir(t)
	project := ir.FromTableSchemas([]*dbschema.TableSchema{
		{
			TableName: "users",
			Columns: []dbschema.ColumnInfo{
				{Name: "id", DBType: "bigint", IsPrimary: true, IsAutoIncr: true},
				{Name: "username", DBType: "varchar(64)", IsNullable: false, Comment: "login name"},
				{Name: "email", DBType: "varchar(128)", IsNullable: false},
			},
		},
	}, "UserAdminService", "useradmin")

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/useradmin",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   false,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "model", "generated_user.go"))
	mustExist(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
	mustContain(t, filepath.Join(outDir, "model", "generated_user.go"), `gorm:"column:username;not null;type:varchar(64)"`)
	mustContain(t, filepath.Join(outDir, "repository", "generated_user_repository.go"), "users.Create")
}

func TestGenerateProject_FromGoIR_GeneratesArtifactsWithoutCompatParseResult(t *testing.T) {
	outDir := newTmpDir(t)
	result, err := parser.ParseFull(filepath.Join("..", "parser", "testdata", "basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	project := ir.FromParseResult(result)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithModel:  true,
		WithConfig: false,
		WithDocs:   true,
		WithSkill:  true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "service", "userservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
	mustExist(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
	mustExist(t, filepath.Join(outDir, "model", "generated_user.go"))
	mustExist(t, filepath.Join(outDir, "repository", "generated_user_repository.go"))
	mustExist(t, filepath.Join(outDir, "README.md"))
	mustExist(t, filepath.Join(outDir, "skill", "skill.go"))

	mustContain(t, filepath.Join(outDir, "service", "userservice", "service.go"), "CreateUser")
	mustContain(t, filepath.Join(outDir, "model", "generated_user.go"), `gorm:"primaryKey;autoIncrement"`)
	mustContain(t, filepath.Join(outDir, "README.md"), "UserService")
}

func TestGenerateProject_FromProtoIR_GeneratesProtoArtifactsWithoutCompatParseResult(t *testing.T) {
	outDir := newTmpDir(t)
	protoPath := filepath.Join(t.TempDir(), "greeter.proto")
	content := `
syntax = "proto3";
package greeter;

service Greeter {
  rpc SayHello (HelloRequest) returns (HelloResponse);
}

message HelloRequest {
  string name = 1;
}

message HelloResponse {
  string message = 1;
}
`
	if err := os.WriteFile(protoPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := parser.ParseProto(protoPath)
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}
	project := ir.FromParseResult(result)

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/greeter",
		Protocols:  []string{"http", "grpc"},
		DBDriver:   "sqlite",
		WithConfig: false,
		WithDocs:   true,
		WithSkill:  true,
	})
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "service", "greeter", "service.go"))
	mustExist(t, filepath.Join(outDir, "transport", "greeter", "transport_grpc.go"))
	mustExist(t, filepath.Join(outDir, "pb", "greeter", "greeter.proto"))
	mustExist(t, filepath.Join(outDir, "README.md"))
	mustExist(t, filepath.Join(outDir, "skill", "skill.go"))

	mustContain(t, filepath.Join(outDir, "pb", "greeter", "greeter.proto"), "rpc SayHello")
	mustContain(t, filepath.Join(outDir, "pb", "greeter", "greeter.proto"), "string name = 1;")
	mustContain(t, filepath.Join(outDir, "README.md"), "protoc --go_out=. --go-grpc_out=.")
}

func TestGenerateIR_FromGoIR_GeneratesArtifacts(t *testing.T) {
	outDir := newTmpDir(t)
	result, err := parser.ParseFull(filepath.Join("..", "parser", "testdata", "basic.go"))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	gen := mustNewGenerator(t, generator.Options{
		OutputDir:  outDir,
		ImportPath: "example.com/basic",
		DBDriver:   "sqlite",
		WithDocs:   true,
		WithSkill:  true,
	})
	if err := gen.GenerateIR(ir.FromParseResult(result)); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}

	mustExist(t, filepath.Join(outDir, "service", "userservice", "service.go"))
	mustExist(t, filepath.Join(outDir, "README.md"))
	mustExist(t, filepath.Join(outDir, "skill", "skill.go"))
}
