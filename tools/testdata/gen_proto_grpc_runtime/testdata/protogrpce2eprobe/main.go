package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	idl "example.com/gen_proto_grpc_runtime/pb"
	genTransport "example.com/gen_proto_grpc_runtime/transport/userservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "127.0.0.1:54106",
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
