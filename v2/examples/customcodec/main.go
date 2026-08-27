// Package main demonstrates a custom data format over HTTP: the body is not
// JSON but a simple length-prefixed binary format, standing in for protobuf or
// MessagePack. The same pattern carries any custom format: two small functions
// become the decode and encode sides, and the error encoder shares the
// content type instead of defaulting to JSON.
//
// Concepts shown:
//   - server.RawBodyCodec turns two pure functions into a transport codec
//   - server.TextErrorEncoder keeps error responses in the same format
//   - apperror classification still decides the status; no framework JSON
//     is forced anywhere on this route
//   - the route mounts through Service.Handle because it is a raw HTTP
//     integration, while /health and /readyz stay available
//
// Run:
//
//	go run ./examples/customcodec
//
// Test with curl:
//
//	# The body is a custom binary format: [1 byte length][payload]
//	printf '\x05world' | curl -X POST http://localhost:8080/shout \
//	     -H "Content-Type: application/x-custom" --data-binary @-
//
//	# Invalid body (missing length byte): 400 in the same format
//	printf '' | curl -i -X POST http://localhost:8080/shout --data-binary @-
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

const contentType = "application/x-custom"

// The wire format: [1 byte length N][N bytes payload]. Deliberately simple so
// the example needs no protoc or codegen; protobuf swaps in by replacing the
// two codec functions.
func unmarshalCustom(body []byte) (any, error) {
	if len(body) < 1 {
		return nil, apperror.New(apperror.KindInvalidArgument, "shout.empty_body", "body must carry a length prefix")
	}
	n := int(body[0])
	payload := body[1:]
	if len(payload) != n {
		return nil, apperror.New(apperror.KindInvalidArgument, "shout.bad_length", "length prefix does not match payload")
	}
	return string(payload), nil
}

func marshalCustom(resp any) ([]byte, error) {
	text, ok := resp.(string)
	if !ok {
		return nil, errors.New("response must be a string")
	}
	if len(text) > 255 {
		return nil, errors.New("response too long for the format")
	}
	return append([]byte{byte(len(text))}, []byte(text)...), nil
}

func shout(_ context.Context, req any) (any, error) {
	name := req.(string)
	if name == "" {
		return nil, apperror.New(apperror.KindInvalidArgument, "shout.name_required", "name is required")
	}
	return "HELLO, " + name + "!", nil
}

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	svc, err := kit.NewHTTP(*httpAddr, kit.WithRequestID())
	if err != nil {
		log.Fatal(err)
	}

	decode, encode := server.RawBodyCodec(unmarshalCustom, marshalCustom, contentType)

	svc.Handle("POST /shout", server.NewServer(
		shout,
		decode,
		encode,
		server.ServerErrorEncoder(server.TextErrorEncoder(contentType)),
	))

	log.Println("customcodec example listening on", *httpAddr)

	host, err := kit.NewHost(kit.WithLifecycle(svc))
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
