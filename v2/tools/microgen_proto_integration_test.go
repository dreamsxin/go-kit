package tools_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMicrogenProtoIntegration(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	microgenPath := microgenMainPath(t)

	t.Run("Proto", func(t *testing.T) {
		outDir := filepath.Join(cwd, "testdata", "gen_proto_integration")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_integration",
			"-protocols", "http,grpc",
			"-prefix", "/api/proto",
			"-openapi",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto failed: %v\n%s", err, out)
		}

		mustExistFile(t, filepath.Join(outDir, "go.mod"))
		mustExistFile(t, filepath.Join(outDir, ".microgen", "manifest.json"))
		mustExistFile(t, filepath.Join(outDir, "service", "userservice", "service.go"))
		mustExistFile(t, filepath.Join(outDir, "endpoint", "userservice", "endpoints.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_http.go"))
		mustExistFile(t, filepath.Join(outDir, "transport", "userservice", "transport_grpc.go"))
		mustExistFile(t, filepath.Join(outDir, "client", "userservice", "demo.go"))
		mustExistFile(t, filepath.Join(outDir, "sdk", "userservicesdk", "client.go"))
		mustExistFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"))
		mustExistFile(t, filepath.Join(outDir, "docs", "docs.go"))
		mustExistFile(t, filepath.Join(outDir, "cmd", "main.go"))
		mustNotExistFile(t, filepath.Join(outDir, "idl.go"))
		mustContainFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"), "string id = 1;")
		mustContainFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"), "string name = 2;")
		mustContainFile(t, filepath.Join(outDir, "pb", "userservice", "userservice.proto"), "string email = 3;")
		mustContainFile(t, filepath.Join(outDir, "README.md"), "protoc --go_out=. --go-grpc_out=.")
		mustContainFile(t, filepath.Join(outDir, "README.md"), "pb/userservice/userservice.proto")
		mustContainFile(t, filepath.Join(outDir, "README.md"), "Review the generated proto contract before generating stubs")
		mustContainFile(t, filepath.Join(outDir, ".microgen", "manifest.json"), `"source": "proto"`)
		mustContainFile(t, filepath.Join(outDir, "cmd", "generated_routes.go"), "/api/proto/userservice")
		mustContainFile(t, filepath.Join(outDir, "docs", "openapi.json"), "/api/proto/userservice")
		mustContainFile(t, filepath.Join(outDir, "cmd", "generated_routes.go"), "/api/proto/userservice")
		validateGeneratedContracts(t, outDir)
		assertGeneratedContractSnapshot(t, "proto", outDir)

		// Build check for the generated code
		buildCmd := exec.Command("go", "build", "./...")
		buildCmd.Dir = outDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Logf("Note: Generated proto code requires protoc, so full build might fail without it. Build output: %s", out)
		}
	})

	t.Run("Proto_ComponentFlow_WhenProtocAvailable", func(t *testing.T) {
		protoc, protocGenGo, protocGenGoGRPC := requireProtoToolchain(t)

		outDir := filepath.Join(cwd, "testdata", "gen_proto_component_flow")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_component_flow",
			"-protocols", "http,grpc",
			"-model=false",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto component flow failed: %v\n%s", err, out)
		}

		protocCmd := exec.Command(protoc,
			"--go_out=.",
			"--go-grpc_out=.",
			"pb/userservice/userservice.proto",
		)
		protocCmd.Dir = outDir
		protocCmd.Env = append(os.Environ(),
			"PATH="+strings.Join([]string{
				filepath.Dir(protoc),
				filepath.Dir(protocGenGo),
				filepath.Dir(protocGenGoGRPC),
				os.Getenv("PATH"),
			}, string(os.PathListSeparator)),
		)
		runCommand(t, protocCmd)

		buildTargets := []string{
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./client/...",
			"./sdk/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		componentProbePkg := writeProtoSvcEndpointTransportProbe(t, outDir, "protocomponentprobe", "example.com/gen_proto_component_flow")
		componentCmd := exec.Command("go", "run", "-mod=mod", componentProbePkg)
		componentCmd.Dir = outDir
		runCommand(t, componentCmd)
	})

	t.Run("Proto_Streaming_GeneratedProject_Builds", func(t *testing.T) {
		protoc, protocGenGo, protocGenGoGRPC := requireProtoToolchain(t)

		outDir := filepath.Join(cwd, "testdata", "gen_proto_server_stream")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "server_stream.proto")
		if err := os.WriteFile(protoFile, []byte(`syntax = "proto3";
package chat;

service ChatService {
  rpc SendMessage (SendMessageRequest) returns (SendMessageResponse);
  rpc WatchMessages (WatchMessagesRequest) returns (stream MessageEvent);
  rpc UploadMessages (stream MessageEvent) returns (UploadSummary);
  rpc Interact (stream MessageEvent) returns (stream MessageEvent);
}

message SendMessageRequest { string body = 1; }
message SendMessageResponse { string id = 1; }
message WatchMessagesRequest { string room_id = 1; }
message MessageEvent { string id = 1; string body = 2; }
message UploadSummary { int32 count = 1; }
`), 0o644); err != nil {
			t.Fatalf("write stream proto: %v", err)
		}
		defer os.Remove(protoFile)

		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_server_stream",
			"-protocols", "http,grpc",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto server stream failed: %v\n%s", err, out)
		}

		mustContainFile(t, filepath.Join(outDir, "pb", "chatservice", "chatservice.proto"), "returns (stream MessageEvent)")
		mustContainFile(t, filepath.Join(outDir, "pb", "chatservice", "chatservice.proto"), "rpc UploadMessages (stream MessageEvent) returns (UploadSummary);")
		mustContainFile(t, filepath.Join(outDir, "pb", "chatservice", "chatservice.proto"), "rpc Interact (stream MessageEvent) returns (stream MessageEvent);")
		mustContainFile(t, filepath.Join(outDir, "service", "chatservice", "service.go"), "send func(idl.MessageEvent) error")
		mustContainFile(t, filepath.Join(outDir, "service", "chatservice", "service.go"), "recv func() (idl.MessageEvent, error)")
		mustContainFile(t, filepath.Join(outDir, "transport", "chatservice", "transport_grpc.go"), "ChatService_WatchMessagesServer")
		mustContainFile(t, filepath.Join(outDir, "transport", "chatservice", "transport_grpc.go"), "ChatService_UploadMessagesServer")
		mustContainFile(t, filepath.Join(outDir, "transport", "chatservice", "transport_grpc.go"), "ChatService_InteractServer")

		protocCmd := exec.Command(protoc,
			"--go_out=.",
			"--go-grpc_out=.",
			"pb/chatservice/chatservice.proto",
		)
		protocCmd.Dir = outDir
		protocCmd.Env = append(os.Environ(),
			"PATH="+strings.Join([]string{
				filepath.Dir(protoc),
				filepath.Dir(protocGenGo),
				filepath.Dir(protocGenGoGRPC),
				os.Getenv("PATH"),
			}, string(os.PathListSeparator)),
		)
		runCommand(t, protocCmd)

		buildTargets := []string{
			"./cmd",
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./client/...",
			"./sdk/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		streamProbePkg := writeProtoStreamingSDKProbe(t, outDir, "protostreamsdkprobe", "example.com/gen_proto_server_stream")
		streamProbeCmd := exec.Command("go", "run", "-mod=mod", streamProbePkg)
		streamProbeCmd.Dir = outDir
		runCommand(t, streamProbeCmd)
	})

	t.Run("Proto_GRPC_GeneratedProject_BuildsAndServesRequests", func(t *testing.T) {
		protoc, protocGenGo, protocGenGoGRPC := requireProtoToolchain(t)

		outDir := filepath.Join(cwd, "testdata", "gen_proto_grpc_runtime")
		os.RemoveAll(outDir)

		protoFile := filepath.Join(cwd, "testdata", "service.proto")
		cmd := exec.Command("go", "run", microgenPath,
			"-idl", protoFile,
			"-out", outDir,
			"-import", "example.com/gen_proto_grpc_runtime",
			"-protocols", "http,grpc",
			"-config=false",
			"-docs=false",
			"-model=false",
			"-db=false",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("microgen proto grpc runtime failed: %v\n%s", err, out)
		}

		protocCmd := exec.Command(protoc,
			"--go_out=.",
			"--go-grpc_out=.",
			"pb/userservice/userservice.proto",
		)
		protocCmd.Dir = outDir
		protocCmd.Env = append(os.Environ(),
			"PATH="+strings.Join([]string{
				filepath.Dir(protoc),
				filepath.Dir(protocGenGo),
				filepath.Dir(protocGenGoGRPC),
				os.Getenv("PATH"),
			}, string(os.PathListSeparator)),
		)
		runCommand(t, protocCmd)

		buildTargets := []string{
			"./cmd",
			"./service/...",
			"./endpoint/...",
			"./transport/...",
			"./client/...",
			"./sdk/...",
		}
		buildCmd := exec.Command("go", append([]string{"build", "-mod=mod"}, buildTargets...)...)
		buildCmd.Dir = outDir
		runCommand(t, buildCmd)

		binName := "microgen_proto_grpc_bin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		serverBuildCmd := exec.Command("go", "build", "-mod=mod", "-o", binName, "./cmd")
		serverBuildCmd.Dir = outDir
		runCommand(t, serverBuildCmd)
		defer os.Remove(filepath.Join(outDir, binName))

		httpAddr := freeTCPAddr(t)
		grpcAddr := freeTCPAddr(t)
		baseURL := "http://" + httpAddr
		runCmd := exec.Command("./"+binName, "-http.addr="+httpAddr, "-grpc.addr="+grpcAddr)
		runCmd.Dir = outDir
		runCmd.Env = os.Environ()
		if err := runCmd.Start(); err != nil {
			t.Fatalf("failed to start generated proto grpc project: %v", err)
		}
		defer killCmd(t, runCmd)

		waitServer(t, baseURL+"/health")
		smokeTest{method: "GET", path: "/health", want: "ok"}.run(t, baseURL)
		expectJSONStatusContains(t, "POST", baseURL+"/createuser", `{"name":"http-user","email":"http@example.com"}`, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))

		grpcProbePkg := writeProtoGRPCE2EProbe(t, outDir, "protogrpce2eprobe", "example.com/gen_proto_grpc_runtime", grpcAddr)
		grpcProbeCmd := exec.Command("go", "run", "-mod=mod", grpcProbePkg)
		grpcProbeCmd.Dir = outDir
		grpcOut := runCommand(t, grpcProbeCmd)
		if !strings.Contains(grpcOut, "CreateUser") {
			t.Fatalf("grpc probe output did not contain CreateUser scaffold error:\n%s", grpcOut)
		}

		demoCmd := exec.Command("go", "run", "./client/userservice/demo.go", "-mode=grpc", "-grpc.addr="+grpcAddr)
		demoCmd.Dir = outDir
		demoOut := runCommand(t, demoCmd)
		if !strings.Contains(demoOut, "Demo completed") {
			t.Fatalf("grpc demo output did not show completion:\n%s", demoOut)
		}
		if !strings.Contains(demoOut, "CreateUser") {
			t.Fatalf("grpc demo output did not exercise generated grpc methods:\n%s", demoOut)
		}
	})

}
