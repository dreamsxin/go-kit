# UserService Service

Generated with `go-kit microgen`.

## Quick Start

```bash

# Review the generated proto contract before generating stubs

# Generate Go stubs from the proto contract first
protoc --go_out=. --go-grpc_out=. pb/userservice/userservice.proto

# Start the service
go run ./cmd/main.go

```

## API Endpoints


## Proto Notes

- `pb/userservice/userservice.proto` is generated from the current service contract and should be reviewed before running `protoc`.
- If any unsupported shape still falls back to `TODO`, complete those message fields before generating stubs.



### UserService


* **GetUser**: `GET /getuser`

* **CreateUser**: `POST /createuser`


