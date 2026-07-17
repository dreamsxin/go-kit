package tools_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/generator"
)

type sdkBehaviorResult struct {
	Requests []sdkBehaviorRequest
	Error    sdkBehaviorError
}

type sdkBehaviorRequest struct {
	Method  string
	Path    string
	Query   map[string][]string
	Body    any
	Headers map[string]string
}

type sdkBehaviorError struct {
	Status int
	Body   string
}

func TestGeneratedSDKBehaviorContract(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Join(cwd, "testdata", "gen_fromdb_sqlite")
	if _, err := os.Stat(filepath.Join(root, "sdk", "typescript", "client.ts")); err != nil {
		t.Skip("generated database fixture is absent; run the contract integration tests first")
	}

	want := readSDKBehaviorResult(t, filepath.Join(cwd, "testdata", "sdk_behavior_contract.json"))
	goResult := runGoSDKBehaviorProbe(t, root)
	typeScriptResult := runTypeScriptSDKBehaviorProbe(t, root)

	assertSDKBehaviorResult(t, "Go", goResult, want)
	assertSDKBehaviorResult(t, "TypeScript", typeScriptResult, want)
}

func runGoSDKBehaviorProbe(t *testing.T, root string) sdkBehaviorResult {
	t.Helper()
	probeDir := filepath.Join(root, "testdata", "sdkbehaviorprobe")
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("create Go SDK behavior probe: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(probeDir) })
	if err := os.WriteFile(filepath.Join(probeDir, "main.go"), []byte(goSDKBehaviorProbe), 0o644); err != nil {
		t.Fatalf("write Go SDK behavior probe: %v", err)
	}

	cmd := exec.Command("go", "run", "-mod=mod", "./testdata/sdkbehaviorprobe")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Go SDK behavior probe: %v\n%s", err, out)
	}
	return decodeSDKBehaviorResult(t, out)
}

func runTypeScriptSDKBehaviorProbe(t *testing.T, root string) sdkBehaviorResult {
	t.Helper()
	npx, err := exec.LookPath("npx")
	if err != nil {
		t.Fatalf("TypeScript SDK behavior contract requires npx on PATH")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("TypeScript SDK behavior contract requires Node.js on PATH")
	}

	sourceDir := filepath.Join(root, "sdk", "typescript")
	buildDir := t.TempDir()
	compile := exec.Command(npx,
		"--yes",
		"--package", "typescript@"+generator.TypeScriptCompilerVersion,
		"tsc",
		"--ignoreConfig",
		"client.ts",
		"--target", "ES2022",
		"--module", "ES2022",
		"--moduleResolution", "Bundler",
		"--lib", "ES2022,DOM,DOM.Iterable",
		"--strict",
		"--skipLibCheck",
		"--outDir", buildDir,
	)
	compile.Dir = sourceDir
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile TypeScript SDK behavior probe: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "package.json"), []byte("{\"type\":\"module\"}\n"), 0o644); err != nil {
		t.Fatalf("write TypeScript probe package metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "probe.mjs"), []byte(typeScriptSDKBehaviorProbe), 0o644); err != nil {
		t.Fatalf("write TypeScript SDK behavior probe: %v", err)
	}

	cmd := exec.Command(node, "probe.mjs")
	cmd.Dir = buildDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run TypeScript SDK behavior probe: %v\n%s", err, out)
	}
	return decodeSDKBehaviorResult(t, out)
}

func readSDKBehaviorResult(t *testing.T, path string) sdkBehaviorResult {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read SDK behavior contract: %v", err)
	}
	return decodeSDKBehaviorResult(t, data)
}

func decodeSDKBehaviorResult(t *testing.T, data []byte) sdkBehaviorResult {
	t.Helper()
	var result sdkBehaviorResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode SDK behavior result: %v\n%s", err, data)
	}
	return result
}

func assertSDKBehaviorResult(t *testing.T, sdk string, got, want sdkBehaviorResult) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	t.Fatalf("%s SDK behavior differs from the shared contract\n--- want\n%s\n--- got\n%s", sdk, wantJSON, gotJSON)
}

const goSDKBehaviorProbe = `package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	idl "example.com/gen_fromdb_sqlite"
	sdk "example.com/gen_fromdb_sqlite/sdk/catalogservicesdk"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type Observation struct {
	Method  string
	Path    string
	Query   map[string][]string
	Body    any
	Headers map[string]string
}

type ErrorObservation struct {
	Status int
	Body   string
}

type Result struct {
	Requests []Observation
	Error    ErrorObservation
}

func main() {
	result := Result{}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		var raw []byte
		if req.Body != nil {
			raw, _ = io.ReadAll(req.Body)
		}
		var body any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				panic(err)
			}
		}
		result.Requests = append(result.Requests, Observation{
			Method: req.Method,
			Path:   req.URL.Path,
			Query:  req.URL.Query(),
			Body:   body,
			Headers: map[string]string{
				"accept":       req.Header.Get("Accept"),
				"content_type": req.Header.Get("Content-Type"),
				"x_request":    req.Header.Get("X-Request"),
				"x_static":     req.Header.Get("X-Static"),
			},
		})

		status := http.StatusOK
		responseBody := "{\"data\":null}"
		if req.URL.Path == "/users" {
			responseBody = "{\"data\":[],\"total\":0,\"page\":2,\"page_size\":25}"
		}
		if req.URL.Path == "/user/999" {
			status = http.StatusUnprocessableEntity
			responseBody = "{\"error\":\"invalid user\"}"
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Request:    req,
		}, nil
	})

	client := sdk.New("https://contract.test",
		sdk.WithHTTPClient(&http.Client{Transport: transport}),
		sdk.WithHeader("X-Static", "shared"),
		sdk.WithRequestHook(func(req *http.Request) { req.Header.Set("X-Request", "shared") }),
	)
	ctx := context.Background()
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	_, err := client.GetUser(ctx, idl.GetUserRequest{ID: 42})
	must(err)
	_, err = client.ListUsers(ctx, idl.ListUsersRequest{Page: 2, PageSize: 25, Keyword: "space value"})
	must(err)
	_, err = client.CreateUser(ctx, idl.CreateUserRequest{Username: "alice", Email: "alice@example.com"})
	must(err)
	username := "updated"
	_, err = client.UpdateUser(ctx, idl.UpdateUserRequest{ID: 7, Username: &username})
	must(err)
	_, err = client.GetUser(ctx, idl.GetUserRequest{ID: 999})
	var apiErr *sdk.APIError
	if !errors.As(err, &apiErr) {
		panic("expected exported APIError")
	}
	result.Error = ErrorObservation{Status: apiErr.StatusCode, Body: apiErr.Body}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}
`

const typeScriptSDKBehaviorProbe = `import { APIClient, APIError } from "./client.js";

const result = { Requests: [], Error: { Status: 0, Body: "" } };

const fetcher = async (input, init = {}) => {
  const url = new URL(String(input));
  const query = {};
  for (const [name, value] of url.searchParams) {
    (query[name] ??= []).push(value);
  }
  const headers = new Headers(init.headers);
  result.Requests.push({
    Method: init.method ?? "GET",
    Path: url.pathname,
    Query: query,
    Body: init.body === undefined ? null : JSON.parse(String(init.body)),
    Headers: {
      accept: headers.get("Accept") ?? "",
      content_type: headers.get("Content-Type") ?? "",
      x_request: headers.get("X-Request") ?? "",
      x_static: headers.get("X-Static") ?? "",
    },
  });

  let status = 200;
  let body = '{"data":null}';
  if (url.pathname === "/users") {
    body = '{"data":[],"total":0,"page":2,"page_size":25}';
  }
  if (url.pathname === "/user/999") {
    status = 422;
    body = '{"error":"invalid user"}';
  }
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => body,
  };
};

const client = new APIClient("https://contract.test", {
  fetch: fetcher,
  headers: { "X-Static": "shared" },
});
const options = { headers: { "X-Request": "shared" } };

await client.catalogService.getUser({ id: 42 }, options);
await client.catalogService.listUsers({ page: 2, page_size: 25, keyword: "space value" }, options);
await client.catalogService.createUser({ username: "alice", email: "alice@example.com" }, options);
await client.catalogService.updateUser({ id: 7, username: "updated" }, options);
try {
  await client.catalogService.getUser({ id: 999 }, options);
  throw new Error("expected APIError");
} catch (error) {
  if (!(error instanceof APIError)) {
    throw error;
  }
  result.Error = { Status: error.status, Body: error.body };
}

process.stdout.write(JSON.stringify(result));
`
