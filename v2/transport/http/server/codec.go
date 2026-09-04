package server

import (
	"context"
	"errors"
	"io"
	"net/http"

	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// DefaultRawBodyBytes caps raw request bodies decoded by RawBodyCodec.
const DefaultRawBodyBytes int64 = 1 << 20

// UnmarshalFunc maps a raw request body into a domain request value.
type UnmarshalFunc func(body []byte) (any, error)

// MarshalFunc maps a domain response value into raw bytes.
type MarshalFunc func(response any) ([]byte, error)

// TextErrorEncoder is an ErrorEncoder that writes errors as plain text with
// the given Content-Type. Use it for non-JSON routes (protobuf, binary, text)
// so error responses share the response format instead of defaulting to JSON.
// Classification and redaction run through the same rules as the JSON encoder:
// StatusCoder, ValidationError, rejection errors, and apperror kinds decide
// the status; transporthttp.PublicMessager chooses the message; a 500 always
// reads "Internal Server Error".
//
//	server.NewServer(ep, decodeProto, encodeProto,
//	    server.ServerErrorEncoder(server.TextErrorEncoder("application/x-protobuf")),
//	)
func TextErrorEncoder(contentType string) ErrorEncoder {
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	return func(ctx context.Context, err error, w http.ResponseWriter) {
		status := httpStatus(err)

		var h transporthttp.Headerer
		if errors.As(err, &h) {
			for k, vals := range h.Headers() {
				for _, v := range vals {
					w.Header().Add(k, v)
				}
			}
		}

		w.Header().Set("Content-Type", contentType)

		message := publicErrorMessage(err, status)

		w.WriteHeader(status)
		_, _ = w.Write([]byte(message))
	}
}

// RawBodyCodec returns a decode/encode pair for a route that speaks a
// non-JSON body format - protobuf, MessagePack, custom binary, or text. The
// codec reads the bounded body, hands it to unmarshal, and writes marshal's
// output with the given Content-Type.
//
// Pair it with a matching error encoder (for example a plain-text or
// format-specific one) so error responses share the content type instead of
// defaulting to JSON.
//
// Example - protobuf over HTTP:
//
//	decode, encode := server.RawBodyCodec(
//	    func(body []byte) (any, error) {
//	        var req pb.HelloRequest
//	        return req, proto.Unmarshal(body, &req)
//	    },
//	    func(resp any) ([]byte, error) {
//	        return proto.Marshal(resp.(proto.Message))
//	    },
//	    "application/x-protobuf",
//	)
//	handler := server.NewServer(ep, decode, encode)
func RawBodyCodec(unmarshal UnmarshalFunc, marshal MarshalFunc, contentType string) (DecodeRequestFunc, EncodeResponseFunc) {
	return RawBodyCodecWithMaxBytes(unmarshal, marshal, contentType, DefaultRawBodyBytes)
}

// RawBodyCodecWithMaxBytes is RawBodyCodec with an explicit request body
// limit. Bodies beyond the limit fail with 413 through the standard error
// encoders.
func RawBodyCodecWithMaxBytes(unmarshal UnmarshalFunc, marshal MarshalFunc, contentType string, maxBodyBytes int64) (DecodeRequestFunc, EncodeResponseFunc) {
	if unmarshal == nil || marshal == nil {
		panic("server: codec functions cannot be nil")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	decode := func(_ context.Context, r *http.Request) (any, error) {
		reader := io.Reader(r.Body)
		if maxBodyBytes > 0 {
			reader = &limitedBodyReader{reader: r.Body, remaining: maxBodyBytes}
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		return unmarshal(body)
	}

	encode := func(_ context.Context, w http.ResponseWriter, response any) error {
		data, err := marshal(response)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", contentType)
		// Honor the same transport contracts as the JSON encoder.
		if headerer, ok := response.(transporthttp.Headerer); ok {
			for k, values := range headerer.Headers() {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
		}
		code := responseStatus(response)
		w.WriteHeader(code)
		if code == http.StatusNoContent {
			return nil
		}
		_, err = w.Write(data)
		return err
	}

	return decode, encode
}
