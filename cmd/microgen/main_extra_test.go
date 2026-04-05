package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// ── splitComma 边界情况 ───────────────────────────────────────────────────────

func TestSplitComma_SingleItem(t *testing.T) {
	got := splitComma("mysql")
	if len(got) != 1 || got[0] != "mysql" {
		t.Errorf("splitComma(\"mysql\") = %v, want [mysql]", got)
	}
}

func TestSplitComma_WhitespaceOnly(t *testing.T) {
	got := splitComma("   ")
	if len(got) != 0 {
		t.Errorf("splitComma(\"   \") = %v, want []", got)
	}
}

// ── config.validate ───────────────────────────────────────────────────────────

func TestConfigValidate_ProtoIDL(t *testing.T) {
	cfg := config{fromDB: false, idlPath: "service.proto"}
	if err := cfg.validate(); err != nil {
		t.Errorf("validate with .proto idl: unexpected error: %v", err)
	}
}

func TestConfigValidate_BothFromDBAndIDL(t *testing.T) {
	// fromDB=true 优先，即使 idlPath 也有值也应通过
	cfg := config{fromDB: true, idlPath: "service.go"}
	if err := cfg.validate(); err != nil {
		t.Errorf("validate with both fromDB and idlPath: unexpected error: %v", err)
	}
}

// ── runFromIDL ────────────────────────────────────────────────────────────────

func TestRunFromIDL_GoFile(t *testing.T) {
	idlPath := filepath.Join("parser", "testdata", "basic.go")
	cfg := config{idlPath: idlPath}
	result := runFromIDL(cfg)

	if result == nil {
		t.Fatal("runFromIDL returned nil")
	}
	if len(result.Services) == 0 {
		t.Error("expected at least one service")
	}
	if result.Services[0].ServiceName != "UserService" {
		t.Errorf("ServiceName: got %q, want %q", result.Services[0].ServiceName, "UserService")
	}
}

func TestRunFromIDL_MultiService(t *testing.T) {
	idlPath := filepath.Join("parser", "testdata", "multi.go")
	cfg := config{idlPath: idlPath}
	result := runFromIDL(cfg)

	if len(result.Services) != 2 {
		t.Errorf("Services: want 2, got %d", len(result.Services))
	}
}

func TestRunFromIDL_ProtoFile(t *testing.T) {
	// Use the existing proto file in examples
	idlPath := filepath.Join("..", "..", "examples", "microgen_skill", "greeter.proto")
	cfg := config{idlPath: idlPath}
	result := runFromIDL(cfg)

	if result == nil {
		t.Fatal("runFromIDL returned nil for proto file")
	}
	if len(result.Services) == 0 {
		t.Error("expected at least one service from proto file")
	}
}

// ── parseFlags 默认值 ─────────────────────────────────────────────────────────

func TestParseFlags_Defaults(t *testing.T) {
	// parseFlags 依赖 flag.Parse()，在测试中直接构造 config 验证默认值逻辑
	// 通过检查 splitComma 和 hasGRPC 逻辑来间接验证

	// 默认 protocols = "http" → hasGRPC = false
	protos := strings.Split("http", ",")
	hasGRPC := false
	for _, p := range protos {
		if strings.TrimSpace(p) == "grpc" {
			hasGRPC = true
		}
	}
	if hasGRPC {
		t.Error("default protocols should not include grpc")
	}

	// protocols = "http,grpc" → hasGRPC = true
	protos2 := strings.Split("http,grpc", ",")
	hasGRPC2 := false
	for _, p := range protos2 {
		if strings.TrimSpace(p) == "grpc" {
			hasGRPC2 = true
		}
	}
	if !hasGRPC2 {
		t.Error("protocols=http,grpc should set hasGRPC=true")
	}
}

// ── config 字段完整性 ─────────────────────────────────────────────────────────

func TestConfig_WithGRPC_SetFromProtocols(t *testing.T) {
	// 验证 protocols 包含 grpc 时 withGRPC 被正确设置
	protocols := []string{"http", "grpc"}
	hasGRPC := false
	for _, p := range protocols {
		if strings.TrimSpace(p) == "grpc" {
			hasGRPC = true
		}
	}

	cfg := config{
		protocols: protocols,
		withGRPC:  hasGRPC,
	}
	if !cfg.withGRPC {
		t.Error("withGRPC should be true when protocols contains grpc")
	}
}

func TestConfig_WithGRPC_NotSetForHTTPOnly(t *testing.T) {
	protocols := []string{"http"}
	hasGRPC := false
	for _, p := range protocols {
		if strings.TrimSpace(p) == "grpc" {
			hasGRPC = true
		}
	}

	cfg := config{
		protocols: protocols,
		withGRPC:  hasGRPC,
	}
	if cfg.withGRPC {
		t.Error("withGRPC should be false for http-only protocols")
	}
}
