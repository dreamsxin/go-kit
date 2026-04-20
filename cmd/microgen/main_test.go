package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/generator"
)

func TestSplitComma(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{",,a,,b,,", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitComma(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitComma(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		cfg  config
		pass bool
	}{
		{config{fromDB: false, idlPath: ""}, false},
		{config{fromDB: true, idlPath: ""}, true},
		{config{fromDB: false, idlPath: "test.go"}, true},
	}
	for i, c := range cases {
		err := c.cfg.validate()
		if (err == nil) != c.pass {
			t.Errorf("case %d: validate() error = %v, want pass = %v", i, err, c.pass)
		}
	}
}

func TestConfigValidateExtend(t *testing.T) {
	cases := []struct {
		name string
		cfg  config
		want string
	}{
		{
			name: "missing idl",
			cfg:  config{},
			want: "requires -idl",
		},
		{
			name: "from db unsupported",
			cfg:  config{fromDB: true, idlPath: "combined.go", appendSvc: "OrderService"},
			want: "does not support -from-db",
		},
		{
			name: "proto unsupported",
			cfg:  config{idlPath: "combined.proto", appendSvc: "OrderService"},
			want: "not supported for -append-service, -append-model, or -append-middleware",
		},
		{
			name: "missing append target",
			cfg:  config{idlPath: "combined.go"},
			want: "requires -append-service <Name>, -append-model <Name>, or -append-middleware <Name[,Name...]>",
		},
		{
			name: "multiple append targets",
			cfg:  config{idlPath: "combined.go", appendSvc: "OrderService", appendModel: "Product"},
			want: "choose either -append-service, -append-model, or -append-middleware",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validateExtend()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateExtend() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestConfigValidateExtend_PassesForSupportedAppendService(t *testing.T) {
	cfg := config{idlPath: "combined.go", appendSvc: "OrderService"}
	if err := cfg.validateExtend(); err != nil {
		t.Fatalf("validateExtend() error = %v, want nil", err)
	}
}

func TestConfigValidateExtend_PassesForSupportedAppendModel(t *testing.T) {
	cfg := config{idlPath: "combined.go", appendModel: "Product"}
	if err := cfg.validateExtend(); err != nil {
		t.Fatalf("validateExtend() error = %v, want nil", err)
	}
}

func TestConfigValidateExtend_PassesForSupportedAppendMiddleware(t *testing.T) {
	cfg := config{idlPath: "combined.go", appendMW: []string{"tracing", "metrics"}}
	if err := cfg.validateExtend(); err != nil {
		t.Fatalf("validateExtend() error = %v, want nil", err)
	}
}

func TestConfigValidateExtend_PassesForCheckMode(t *testing.T) {
	cfg := config{outputDir: "project", checkOnly: true}
	if err := cfg.validateExtend(); err != nil {
		t.Fatalf("validateExtend() error = %v, want nil", err)
	}
}

func TestConfigValidateExtend_RejectsCheckWithMutation(t *testing.T) {
	cfg := config{outputDir: "project", checkOnly: true, appendSvc: "OrderService"}
	err := cfg.validateExtend()
	if err == nil || !strings.Contains(err.Error(), "-check cannot be combined") {
		t.Fatalf("validateExtend() error = %v, want check/mutation guidance", err)
	}
}

func TestNewExtendFlagSetUsage(t *testing.T) {
	var out bytes.Buffer
	fs := newExtendFlagSet(&out)
	_ = parseConfig(fs, nil)
	fs.Usage()

	usage := out.String()
	for _, want := range []string{
		"Usage of microgen extend:",
		"-append-service <Name>",
		"-append-model <Name>",
		"-append-middleware <Name[,Name...]>",
		"-check -out <project>",
		"supported: tracing,error-handling,metrics",
		"full combined Go IDL input only",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("extend usage missing %q:\n%s", want, usage)
		}
	}
}

func TestPrintExtendCheckReport(t *testing.T) {
	var out bytes.Buffer
	printExtendCheckReport(&out, &generator.ExistingProject{
		Root:       "D:/work/demo",
		ModulePath: "example.com/demo",
		Services: []generator.ExistingService{
			{Name: "UserService", PackageName: "userservice"},
		},
		Models: []generator.ExistingModel{
			{Name: "User"},
		},
		AggregationPoints: generator.AggregationPoints{
			GeneratedServices: "D:/work/demo/cmd/generated_services.go",
			GeneratedRoutes:   "D:/work/demo/cmd/generated_routes.go",
		},
		Ownership: map[string]generator.FileOwnership{
			"service/userservice/generated_repos.go":          {Tier: generator.OwnershipGeneratorRebuildable},
			"endpoint/userservice/generated_chain.go":         {Tier: generator.OwnershipGeneratorRebuildable},
			"cmd/generated_services.go":                       {Tier: generator.OwnershipGeneratorAggregation},
			"cmd/generated_routes.go":                         {Tier: generator.OwnershipGeneratorAggregation},
			"cmd/main.go":                                     {Tier: generator.OwnershipUserProtected},
		},
		Warnings: []string{"cmd/main.go is treated as protected and will not be rewritten by extend mode"},
		Features: generator.ExistingProjectFeatures{
			WithModel:            true,
			GeneratedMiddlewares: []string{"tracing"},
		},
	})

	report := out.String()
	for _, want := range []string{
		"Extend compatibility for D:/work/demo",
		"Summary:",
		"- Module: example.com/demo",
		"- Overall status: ready",
		"- Generated middleware: tracing",
		"Compatibility Seams:",
		"- Generated services seam: cmd/generated_services.go",
		"- Generated routes seam: cmd/generated_routes.go",
		"- Generated runtime seam: missing",
		"Append Paths:",
		"- append-service: ready",
		"- append-model: ready",
		"- append-middleware: ready",
		"Warnings:",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("extend report missing %q:\n%s", want, report)
		}
	}
}

func TestPrintExtendCheckReport_ShowsMissingSeamsPerAppendPath(t *testing.T) {
	var out bytes.Buffer
	printExtendCheckReport(&out, &generator.ExistingProject{
		Root:       "D:/work/legacy",
		ModulePath: "example.com/legacy",
		Services: []generator.ExistingService{
			{Name: "UserService", PackageName: "userservice"},
		},
		Ownership: map[string]generator.FileOwnership{
			"cmd/main.go": {Tier: generator.OwnershipUserProtected},
		},
		Features: generator.ExistingProjectFeatures{
			WithModel: true,
			WithDB:    true,
		},
	})

	report := out.String()
	for _, want := range []string{
		"- Overall status: needs compatibility seams",
		"- append-service: needs compatibility seams (missing: cmd/generated_services.go, cmd/generated_routes.go)",
		"- append-model: needs compatibility seams (missing: cmd/generated_services.go, cmd/generated_runtime.go, service/userservice/generated_repos.go)",
		"- append-middleware: needs compatibility seams (missing: cmd/generated_routes.go, endpoint/userservice/generated_chain.go)",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("extend report missing %q:\n%s", want, report)
		}
	}
}

func TestExtendCheckExitCode(t *testing.T) {
	ready := &generator.ExistingProject{
		Services: []generator.ExistingService{{Name: "UserService", PackageName: "userservice"}},
		AggregationPoints: generator.AggregationPoints{
			GeneratedServices: "cmd/generated_services.go",
			GeneratedRoutes:   "cmd/generated_routes.go",
		},
		Ownership: map[string]generator.FileOwnership{
			"service/userservice/generated_repos.go":  {Tier: generator.OwnershipGeneratorRebuildable},
			"endpoint/userservice/generated_chain.go": {Tier: generator.OwnershipGeneratorRebuildable},
		},
		Features: generator.ExistingProjectFeatures{
			WithModel: true,
		},
	}
	if code := extendCheckExitCode(ready); code != 0 {
		t.Fatalf("extendCheckExitCode(ready) = %d, want 0", code)
	}

	notReady := &generator.ExistingProject{
		Services: []generator.ExistingService{{Name: "UserService", PackageName: "userservice"}},
		Features: generator.ExistingProjectFeatures{
			WithModel: true,
			WithDB:    true,
		},
	}
	if code := extendCheckExitCode(notReady); code != 2 {
		t.Fatalf("extendCheckExitCode(notReady) = %d, want 2", code)
	}
}
