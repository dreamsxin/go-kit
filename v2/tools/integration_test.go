package tools_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// TestAllExamples builds and runs all examples to ensure they still work.
func TestAllExamples(t *testing.T) {
	cwd, _ := os.Getwd()
	root := filepath.Dir(cwd)
	examplesDir := filepath.Join(root, "examples")

	examples := []struct {
		name       string
		path       string
		port       int
		run        bool // whether to run and smoke test
		smokeTests []smokeTest
	}{
		{
			name: "quickstart",
			path: "quickstart",
			port: 8082,
			run:  true,
			smokeTests: []smokeTest{
				{method: "POST", path: "/hello", body: `{"name":"world"}`, want: "Hello, world!"},
			},
		},
		{
			name: "best_practice",
			path: "best_practice",
			port: 8083,
			run:  true,
			smokeTests: []smokeTest{
				{method: "POST", path: "/hello", body: `{"name":"Alice"}`, want: "Hello, Alice!"},
			},
		},
		{
			name: "microgen_skill",
			path: "microgen_skill",
			port: 8084,
			run:  true,
			smokeTests: []smokeTest{
				{method: "POST", path: "/sayhello", body: `{"name":"Bob", "tags":["test"]}`, want: "Hello, Bob!"},
				{method: "GET", path: "/skill", want: "SayHello"},
				{method: "GET", path: "/skill?format=openai", want: "\"function\""},
				{method: "GET", path: "/skill?format=mcp", want: "inputSchema"},
				{method: "GET", path: "/skill?format=unknown", want: "\"function\""},
			},
		},
	}

	for _, tc := range examples {
		t.Run(tc.name, func(t *testing.T) {
			pkgPath := filepath.Join(examplesDir, tc.path)

			// 1. Build check
			t.Logf("Building %s...", tc.name)
			binName := "test_bin_" + tc.name
			if runtime.GOOS == "windows" {
				binName += ".exe"
			}

			buildPath := "."
			if tc.name == "microgen_skill" {
				buildPath = "./cmd"
			}

			// Ensure dependencies are tidy
			tidyCmd := exec.Command("go", "mod", "tidy")
			tidyCmd.Dir = pkgPath
			tidyCmd.Run()

			cmd := exec.Command("go", "build", "-o", binName, buildPath)
			cmd.Dir = pkgPath
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build failed: %v\n%s", err, out)
			}
			defer os.Remove(filepath.Join(pkgPath, binName))

			if !tc.run {
				return
			}

			// 2. Run and smoke test
			t.Logf("Running %s on port %d...", tc.name, tc.port)
			addr := fmt.Sprintf(":%d", tc.port)
			baseURL := fmt.Sprintf("http://localhost:%d", tc.port)

			runCmd := exec.Command("./" + binName)
			if tc.name == "microgen_skill" {
				runCmd.Args = append(runCmd.Args, "-http.addr="+addr, "-grpc.addr=:8091")
			} else {
				runCmd.Args = append(runCmd.Args, "-http.addr="+addr)
			}
			runCmd.Dir = pkgPath
			runCmd.Env = os.Environ()

			if err := runCmd.Start(); err != nil {
				t.Fatalf("failed to start %s: %v", tc.name, err)
			}

			killProcess := func() {
				if runCmd.Process != nil {
					if runtime.GOOS == "windows" {
						exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", runCmd.Process.Pid)).Run()
					} else {
						runCmd.Process.Kill()
					}
				}
			}
			defer killProcess()

			// Wait for server to start
			waitServer(t, baseURL+"/health")

			for _, st := range tc.smokeTests {
				st.run(t, baseURL)
			}
		})
	}
}

type smokeTest struct {
	method string
	path   string
	body   string
	want   string
}

func (st smokeTest) run(t *testing.T, baseUrl string) {
	url := baseUrl + st.path
	req, _ := http.NewRequest(st.method, url, strings.NewReader(st.body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("%s %s failed: %v", st.method, st.path, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("%s %s: want 200, got %d", st.method, st.path, resp.StatusCode)
	}

	var data map[string]any
	json.NewDecoder(resp.Body).Decode(&data)
	body, _ := json.Marshal(data)
	if !strings.Contains(string(body), st.want) {
		t.Errorf("%s %s: body %s does not contain %q", st.method, st.path, body, st.want)
	}
}

func expectJSONStatusContains(t *testing.T, method, url, body string, wantStatus int, want string) {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: build request: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want status %d, got %d", method, url, wantStatus, resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("%s %s: decode json body: %v", method, url, err)
	}
	encoded, _ := json.Marshal(data)
	if !strings.Contains(string(encoded), want) {
		t.Fatalf("%s %s: body %s does not contain %q", method, url, encoded, want)
	}
}

func expectStatusContains(t *testing.T, method, url, body string, wantStatus int, want string) {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: build request: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want status %d, got %d", method, url, wantStatus, resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, url, err)
	}
	if !strings.Contains(string(respBody), want) {
		t.Fatalf("%s %s: body %s does not contain %q", method, url, respBody, want)
	}
}

func runCommand(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return string(out)
}

func resolveCommandPath(name string, envVar string, extraDirs ...string) string {
	if envVar != "" {
		if candidate := strings.TrimSpace(os.Getenv(envVar)); candidate != "" {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	candidates := []string{
		filepath.Join("D:\\gowork\\bin", name),
		filepath.Join(os.Getenv("USERPROFILE"), "go", "bin", name),
	}
	candidates = append(candidates, extraDirs...)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func requireProtoToolchain(t *testing.T) (string, string, string) {
	t.Helper()

	protoc := resolveCommandPath("protoc.exe", "PROTOC", filepath.Join("D:\\EasyDiffusion\\installer_files\\env\\Lib\\site-packages\\torch\\bin", "protoc.exe"))
	protocGenGo := resolveCommandPath("protoc-gen-go.exe", "PROTOC_GEN_GO")
	protocGenGoGRPC := resolveCommandPath("protoc-gen-go-grpc.exe", "PROTOC_GEN_GO_GRPC")

	var missing []string
	if protoc == "" {
		missing = append(missing, "protoc")
	}
	if protocGenGo == "" {
		missing = append(missing, "protoc-gen-go")
	}
	if protocGenGoGRPC == "" {
		missing = append(missing, "protoc-gen-go-grpc")
	}
	if len(missing) > 0 {
		t.Skipf("protobuf toolchain incomplete; missing %s", strings.Join(missing, ", "))
	}
	return protoc, protocGenGo, protocGenGoGRPC
}

func writeIDLSvcEndpointTransportProbe(t *testing.T, outDir, probeDirName, importPath string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	idl "` + importPath + `"
	userserviceendpoint "` + importPath + `/endpoint/userservice"
	userservicesvc "` + importPath + `/service/userservice"
	userservicetransport "` + importPath + `/transport/userservice"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

func main() {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		panic(err)
	}

	svc := userservicesvc.NewService(nil)
	endpoints := userserviceendpoint.MakeServerEndpoints(svc, logger)
	handler := userservicetransport.NewHTTPHandler(endpoints)

	reqBody := []byte(` + "`" + `{"username":"component-user","email":"component@example.com"}` + "`" + `)
	req := httptest.NewRequest(http.MethodPost, "/createuser", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		panic("unexpected status")
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "Internal Server Error") {
		panic("unexpected body")
	}

	_ = idl.CreateUserRequest{}
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func writeProtoSvcEndpointTransportProbe(t *testing.T, outDir, probeDirName, importPath string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	idl "` + importPath + `/pb"
	userserviceendpoint "` + importPath + `/endpoint/userservice"
	userservicesvc "` + importPath + `/service/userservice"
	userservicetransport "` + importPath + `/transport/userservice"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

func main() {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		panic(err)
	}

	svc := userservicesvc.NewService(nil)
	endpoints := userserviceendpoint.MakeServerEndpoints(svc, logger)
	handler := userservicetransport.NewHTTPHandler(endpoints)

	reqBody := []byte(` + "`" + `{"name":"proto-user","email":"proto@example.com"}` + "`" + `)
	req := httptest.NewRequest(http.MethodPost, "/createuser", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		panic("unexpected status")
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "Internal Server Error") {
		panic("unexpected body")
	}

	_ = idl.CreateUserRequest{}
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func writeProtoGRPCE2EProbe(t *testing.T, outDir, probeDirName, importPath, grpcAddr string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	idl "` + importPath + `/pb"
	genTransport "` + importPath + `/transport/userservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "` + grpcAddr + `",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		panic(fmt.Sprintf("dial grpc: %v", err))
	}
	defer conn.Close()

	createUser := genTransport.NewGRPCCreateUserClient(conn)
	_, err = createUser(context.Background(), idl.CreateUserRequest{
		Name:  "grpc-e2e",
		Email: "grpc-e2e@example.com",
	})
	if err == nil {
		panic("expected scaffold grpc error")
	}
	if !strings.Contains(err.Error(), "CreateUser") {
		panic(fmt.Sprintf("unexpected grpc error: %v", err))
	}
	fmt.Println(err.Error())
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func writeProtoStreamingSDKProbe(t *testing.T, outDir, probeDirName, importPath string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	idl "` + importPath + `/pb"
	chatendpoint "` + importPath + `/endpoint/chatservice"
	chatsdk "` + importPath + `/sdk/chatservicesdk"
	chatsvc "` + importPath + `/service/chatservice"
	chattransport "` + importPath + `/transport/chatservice"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

type streamSvc struct{}

func (streamSvc) SendMessage(ctx context.Context, req idl.SendMessageRequest) (idl.SendMessageResponse, error) {
	return idl.SendMessageResponse{Id: "sent:" + req.Body}, nil
}

func (streamSvc) WatchMessages(ctx context.Context, req idl.WatchMessagesRequest, send func(idl.MessageEvent) error) error {
	if req.RoomId == "server-error" {
		return fmt.Errorf("watch server error")
	}
	if req.RoomId == "slow" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			return nil
		}
	}
	if req.RoomId == "backpressure" {
		if err := send(idl.MessageEvent{Id: "1", Body: "first"}); err != nil {
			return err
		}
		return send(idl.MessageEvent{Id: "2", Body: "second"})
	}
	if req.RoomId == "deadline" {
		if err := send(idl.MessageEvent{Id: "1", Body: "first"}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return send(idl.MessageEvent{Id: "2", Body: "late"})
		}
	}
	if req.RoomId != "room-1" {
		return fmt.Errorf("unexpected room %q", req.RoomId)
	}
	if err := send(idl.MessageEvent{Id: "1", Body: "hello"}); err != nil {
		return err
	}
	return send(idl.MessageEvent{Id: "2", Body: "world"})
}

func (streamSvc) UploadMessages(ctx context.Context, recv func() (idl.MessageEvent, error)) (idl.UploadSummary, error) {
	var count int32
	for {
		event, err := recv()
		if errors.Is(err, io.EOF) {
			return idl.UploadSummary{Count: count}, nil
		}
		if err != nil {
			return idl.UploadSummary{}, err
		}
		if event.Body == "server-error" {
			return idl.UploadSummary{}, fmt.Errorf("upload server error")
		}
		count++
	}
}

func (streamSvc) Interact(ctx context.Context, recv func() (idl.MessageEvent, error), send func(idl.MessageEvent) error) error {
	for {
		event, err := recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if event.Body == "server-error" {
			return fmt.Errorf("interact server error")
		}
		if err := send(idl.MessageEvent{Id: event.Id, Body: "echo:" + event.Body}); err != nil {
			return err
		}
	}
}

func main() {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		panic(err)
	}

	svc := streamSvc{}
	endpoints := chatendpoint.MakeServerEndpoints(svc, logger)
	server := grpc.NewServer()
	chattransport.RegisterGRPCServer(server, svc, endpoints)

	lis := bufconn.Listen(1024 * 1024)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			panic(err)
		}
	}()
	defer func() {
		server.Stop()
		_ = lis.Close()
		wg.Wait()
	}()

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client := chatsdk.NewGRPCStreaming(conn)
	var watched []string
	if err := client.WatchMessages(context.Background(), idl.WatchMessagesRequest{RoomId: "room-1"}, func(event idl.MessageEvent) error {
		watched = append(watched, event.Body)
		return nil
	}); err != nil {
		panic(err)
	}
	if len(watched) != 2 || watched[0] != "hello" || watched[1] != "world" {
		panic(fmt.Sprintf("unexpected watch events: %#v", watched))
	}

	uploadInputs := []idl.MessageEvent{{Id: "1", Body: "a"}, {Id: "2", Body: "b"}}
	uploadIndex := 0
	summary, err := client.UploadMessages(context.Background(), func() (idl.MessageEvent, error) {
		if uploadIndex >= len(uploadInputs) {
			return idl.MessageEvent{}, io.EOF
		}
		event := uploadInputs[uploadIndex]
		uploadIndex++
		return event, nil
	})
	if err != nil {
		panic(err)
	}
	if summary.Count != 2 {
		panic(fmt.Sprintf("unexpected upload count: %d", summary.Count))
	}

	interactInputs := []idl.MessageEvent{{Id: "x", Body: "ping"}}
	interactIndex := 0
	var echoed []string
	if err := client.Interact(context.Background(), func() (idl.MessageEvent, error) {
		if interactIndex >= len(interactInputs) {
			return idl.MessageEvent{}, io.EOF
		}
		event := interactInputs[interactIndex]
		interactIndex++
		return event, nil
	}, func(event idl.MessageEvent) error {
		echoed = append(echoed, event.Body)
		return nil
	}); err != nil {
		panic(err)
	}
	if len(echoed) != 1 || echoed[0] != "echo:ping" {
		panic(fmt.Sprintf("unexpected bidi events: %#v", echoed))
	}

	expectErrContains := func(name string, err error, want string) {
		if err == nil {
			panic(name + ": expected error")
		}
		if !strings.Contains(err.Error(), want) {
			panic(fmt.Sprintf("%s: error %q does not contain %q", name, err.Error(), want))
		}
	}

	err = client.WatchMessages(context.Background(), idl.WatchMessagesRequest{RoomId: "server-error"}, func(idl.MessageEvent) error {
		return nil
	})
	expectErrContains("watch server error", err, "watch server error")

	err = client.WatchMessages(context.Background(), idl.WatchMessagesRequest{RoomId: "room-1"}, func(idl.MessageEvent) error {
		return fmt.Errorf("watch callback error")
	})
	expectErrContains("watch callback error", err, "watch callback error")

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.WatchMessages(cancelCtx, idl.WatchMessagesRequest{RoomId: "slow"}, func(idl.MessageEvent) error {
		return nil
	})
	expectErrContains("watch cancellation", err, "canceled")

	var slowMu sync.Mutex
	var slowEvents []string
	err = client.WatchMessages(context.Background(), idl.WatchMessagesRequest{RoomId: "backpressure"}, func(event idl.MessageEvent) error {
		slowMu.Lock()
		slowEvents = append(slowEvents, event.Body)
		if len(slowEvents) == 1 {
			time.Sleep(50 * time.Millisecond)
			if len(slowEvents) != 1 {
				slowMu.Unlock()
				return fmt.Errorf("stream callback was re-entered before first callback returned")
			}
		}
		slowMu.Unlock()
		return nil
	})
	if err != nil {
		panic(err)
	}
	if len(slowEvents) != 2 || slowEvents[0] != "first" || slowEvents[1] != "second" {
		panic(fmt.Sprintf("unexpected slow callback events: %#v", slowEvents))
	}

	slowCtx, slowCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer slowCancel()
	var deadlineEvents []string
	err = client.WatchMessages(slowCtx, idl.WatchMessagesRequest{RoomId: "deadline"}, func(event idl.MessageEvent) error {
		deadlineEvents = append(deadlineEvents, event.Body)
		time.Sleep(80 * time.Millisecond)
		return nil
	})
	expectErrContains("watch slow consumer deadline", err, "DeadlineExceeded")
	if len(deadlineEvents) != 1 || deadlineEvents[0] != "first" {
		panic(fmt.Sprintf("unexpected deadline events: %#v", deadlineEvents))
	}

	_, err = client.UploadMessages(context.Background(), func() (idl.MessageEvent, error) {
		return idl.MessageEvent{}, fmt.Errorf("upload recv error")
	})
	expectErrContains("upload recv error", err, "upload recv error")

	uploadServerErrorInputs := []idl.MessageEvent{{Id: "1", Body: "server-error"}}
	uploadServerErrorIndex := 0
	_, err = client.UploadMessages(context.Background(), func() (idl.MessageEvent, error) {
		if uploadServerErrorIndex >= len(uploadServerErrorInputs) {
			return idl.MessageEvent{}, io.EOF
		}
		event := uploadServerErrorInputs[uploadServerErrorIndex]
		uploadServerErrorIndex++
		return event, nil
	})
	expectErrContains("upload server error", err, "upload server error")

	interactRecvErrInputs := []idl.MessageEvent{{Id: "1", Body: "first"}}
	interactRecvErrIndex := 0
	err = client.Interact(context.Background(), func() (idl.MessageEvent, error) {
		if interactRecvErrIndex >= len(interactRecvErrInputs) {
			return idl.MessageEvent{}, fmt.Errorf("interact recv error")
		}
		event := interactRecvErrInputs[interactRecvErrIndex]
		interactRecvErrIndex++
		return event, nil
	}, func(idl.MessageEvent) error { return nil })
	expectErrContains("interact recv error", err, "interact recv error")

	interactSendErrInputs := []idl.MessageEvent{{Id: "1", Body: "first"}}
	interactSendErrIndex := 0
	err = client.Interact(context.Background(), func() (idl.MessageEvent, error) {
		if interactSendErrIndex >= len(interactSendErrInputs) {
			return idl.MessageEvent{}, io.EOF
		}
		event := interactSendErrInputs[interactSendErrIndex]
		interactSendErrIndex++
		return event, nil
	}, func(idl.MessageEvent) error { return fmt.Errorf("interact send callback error") })
	expectErrContains("interact send callback error", err, "interact send callback error")

	interactServerErrorInputs := []idl.MessageEvent{{Id: "1", Body: "server-error"}}
	interactServerErrorIndex := 0
	err = client.Interact(context.Background(), func() (idl.MessageEvent, error) {
		if interactServerErrorIndex >= len(interactServerErrorInputs) {
			return idl.MessageEvent{}, io.EOF
		}
		event := interactServerErrorInputs[interactServerErrorIndex]
		interactServerErrorIndex++
		return event, nil
	}, func(idl.MessageEvent) error { return nil })
	expectErrContains("interact server error", err, "interact server error")

	_ = chatsvc.NewService
}
`
	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write %s: %v", probeDirName, err)
	}
	return "./testdata/" + probeDirName
}

func writeConfigRemoteProbe(t *testing.T, outDir, probeDirName, importPath string) string {
	t.Helper()

	probeDir := filepath.Join(outDir, "testdata", probeDirName)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", probeDirName, err)
	}

	probePath := filepath.Join(probeDir, "main.go")
	probe := `package main

import (
	"fmt"
	"os"

	"` + importPath + `/config"
)

func main() {
	if len(os.Args) != 2 {
		panic("expected config path")
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", cfg.Server.HTTPAddr)
	fmt.Printf("%s\n", cfg.Logging.Level)
}
`

	if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
		t.Fatalf("write remote config probe: %v", err)
	}

	return "./testdata/" + probeDirName
}

func createSQLiteSchema(t *testing.T, dbPath string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir sqlite dir: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	schema := `
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO users (username, email) VALUES ('seed-user', 'seed@example.com');
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("init sqlite schema: %v", err)
	}
}

func waitServer(t *testing.T, url string) {
	for i := 0; i < 20; i++ {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s failed to start in time", url)
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}

func killCmd(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
		return
	}
	_ = cmd.Process.Kill()
}

func mustExistFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}

func mustNotExistFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected file not to exist: %s", path)
	}
}

func mustContainFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("expected file %s to contain %q", path, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}
