package main

	import (
		"context"
		"fmt"
		"os"

		idl "example.com/gen_idl_components"
		userservicesdk "example.com/gen_idl_components/sdk/userservicesdk"
	)

	func main() {
		client := userservicesdk.New(os.Getenv("SDK_BASE_URL"))
		_, err := client.CreateUser(context.Background(), idl.CreateUserRequest{
			Username: "sdk-user",
			Email:    "sdk@example.com",
		})
		if err == nil {
			panic("expected generated sdk call to surface scaffold error")
		}
		fmt.Println(err.Error())
	}
