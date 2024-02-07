package grpc

import (
	"net"
	"testing"

	"google.golang.org/grpc"

	test "github.com/dreamsxin/go-kit/examples/transport/_grpc_test"
	"github.com/dreamsxin/go-kit/examples/transport/_grpc_test/pb"
)

const (
	hostPort string = "localhost:8002"
)

func TestGRPCServer(t *testing.T) {
	var (
		server  = grpc.NewServer()
		service = test.NewService()
	)

	sc, err := net.Listen("tcp", hostPort)
	if err != nil {
		t.Fatalf("unable to listen: %+v", err)
	}
	defer server.GracefulStop()

	pb.RegisterTestServer(server, test.NewBinding(service))
	_ = server.Serve(sc)
}
