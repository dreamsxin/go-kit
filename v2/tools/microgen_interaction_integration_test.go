package tools_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMicrogenInteractionIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Dir(cwd)
	microgenPath := microgenMainPath(t)

	outDir := filepath.Join(cwd, "testdata", "gen_interaction_e2e")
	os.RemoveAll(outDir)

	idlFile := filepath.Join(root, "cmd", "microgen", "parser", "testdata", "basic.go")
	cmd := exec.Command("go", "run", microgenPath,
		"-idl", idlFile,
		"-out", outDir,
		"-import", "example.com/gen_interaction_e2e",
		"-config=false",
		"-docs=false",
		"-model=false",
		"-db=false",
		"-interaction=true",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("microgen interaction e2e failed: %v\n%s", err, out)
	}

	// Verify generated files
	mustExistFile(t, filepath.Join(outDir, "cmd", "generated_interaction.go"))
	mustExistFile(t, filepath.Join(outDir, ".ai", "PROJECT_GUIDE.md"))

	// Build
	binName := "microgen_interaction_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
	buildCmd.Dir = outDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("generated interaction project build failed: %v\n%s", err, out)
	}
	binPath := filepath.Join(outDir, binName)
	defer os.Remove(binPath)

	// Start server
	httpAddr := freeTCPAddr(t)
	baseURL := "http://" + httpAddr
	runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr)
	runCmd.Dir = outDir
	runCmd.Env = os.Environ()
	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start generated project: %v", err)
	}
	defer killCmd(t, runCmd)

	waitServer(t, baseURL+"/health")

	// Smoke tests for discovery endpoints
	smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
	expectStatusContains(t, "GET", baseURL+"/debug/routes", "", http.StatusOK, "/mcp")

	// MCP initialize — StreamableHandler issues a Mcp-Session-Id header that
	// must be used for all subsequent requests on the same session.
	var mcpSessionID string
	t.Run("MCP_Initialize", func(t *testing.T) {
		sid, resp := postMCPWithSession(t, baseURL+"/mcp", "", map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
			},
		})
		if sid == "" {
			t.Fatalf("expected Mcp-Session-Id header on initialize, got empty")
		}
		mcpSessionID = sid
		postMCPNotification(t, baseURL+"/mcp", sid, "notifications/initialized")
		result := resp["result"].(map[string]any)
		serverInfo := result["serverInfo"].(map[string]any)
		if serverInfo["name"] != "go-kit interaction" {
			t.Fatalf("unexpected serverInfo.name: %v", serverInfo["name"])
		}
		caps := result["capabilities"].(map[string]any)
		if _, ok := caps["tools"]; !ok {
			t.Fatalf("capabilities should contain tools: %+v", caps)
		}
	})

	// MCP tools/list
	t.Run("MCP_ToolsList", func(t *testing.T) {
		_, resp := postMCPWithSession(t, baseURL+"/mcp", mcpSessionID, map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
		})
		result := resp["result"].(map[string]any)
		tools := result["tools"].([]any)
		if len(tools) == 0 {
			t.Fatalf("expected tools, got none")
		}
		toolNames := make(map[string]bool)
		for _, tt := range tools {
			tool := tt.(map[string]any)
			toolNames[tool["name"].(string)] = true
		}
		expected := []string{"CreateUser", "GetUser", "ListUsers", "DeleteUser", "UpdateUser"}
		for _, name := range expected {
			if !toolNames[name] {
				t.Fatalf("expected tool %q in list, got %+v", name, toolNames)
			}
		}
	})

	// MCP tools/call — verify the call reaches the endpoint layer. Tool execution
	// failures are represented by a CallToolResult with isError=true.
	t.Run("MCP_ToolsCall_ReachesEndpoint", func(t *testing.T) {
		_, resp := postMCPWithSession(t, baseURL+"/mcp", mcpSessionID, map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "CreateUser",
				"arguments": map[string]any{"username": "alice", "email": "alice@example.com"},
			},
		})
		if resp["error"] != nil {
			t.Fatalf("execution failure must not be a JSON-RPC error: %+v", resp["error"])
		}
		result := resp["result"].(map[string]any)
		if result["isError"] != true {
			t.Fatalf("expected isError=true, got %+v", result)
		}
		content := result["content"].([]any)[0].(map[string]any)
		if !strings.Contains(content["text"].(string), "not implemented") {
			t.Fatalf("expected 'not implemented' in tool result, got: %+v", content)
		}
	})

	expectStatusContains(t, "GET", baseURL+"/skill", "", http.StatusNotFound, "404 page not found")
}

func postMCP(t *testing.T, url string, body map[string]any) map[string]any {
	t.Helper()
	_, resp := postMCPWithSession(t, url, "", body)
	return resp
}

func postMCPWithSession(t *testing.T, url, sessionID string, body map[string]any) (string, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: want 200, got %d", url, resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	return resp.Header.Get("Mcp-Session-Id"), result
}

func postMCPNotification(t *testing.T, url, sessionID, method string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST notification %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST notification %s: want 202, got %d", url, resp.StatusCode)
	}
}
