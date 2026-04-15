package main

import (
	"context"
	"fmt"

	idl "example.com/gen_idl_components"
	userservicesdk "example.com/gen_idl_components/sdk/userservicesdk"
)

func main() {
	client := userservicesdk.New("http://127.0.0.1:60262")
	_, err := client.CreateUser(context.Background(), idl.CreateUserRequest{
		Username: "sdk-user",
		Email:    "sdk@example.com",
	})
	if err == nil {
		panic("expected generated sdk call to surface scaffold error")
	}
	fmt.Println(err.Error())
}
