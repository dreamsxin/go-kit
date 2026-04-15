module example.com/gen_proto_component_flow

go 1.25.8

require (
	github.com/dreamsxin/go-kit v0.0.0
	github.com/gorilla/mux v1.8.1
	github.com/sony/gobreaker v1.0.0
	golang.org/x/time v0.15.0
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
	gorm.io/gorm v1.31.1
)

require (
	github.com/streadway/handy v0.0.0-20200128134331-0f66f006fb2e // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
)

replace github.com/dreamsxin/go-kit => ../../../
