package client

import (
	"context"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc/metadata"
)

// RequestFunc is called before the gRPC request is sent.  It receives the
// outgoing metadata map and may add or modify entries.
type RequestFunc func(context.Context, *metadata.MD) context.Context

// SetRequestHeader returns a RequestFunc that adds a single key/value pair
// to the outgoing gRPC metadata.  Binary headers (suffix "-bin") are
// automatically base64-encoded.
func SetRequestHeader(key, val string) RequestFunc {
	return func(ctx context.Context, md *metadata.MD) context.Context {
		key, val := EncodeKeyValue(key, val)
		(*md)[key] = append((*md)[key], val)
		return ctx
	}
}

const (
	binHdrSuffix = "-bin"
)

// EncodeKeyValue normalises a metadata key to lowercase and base64-encodes
// the value if the key has the "-bin" suffix.
func EncodeKeyValue(key, val string) (string, string) {
	key = strings.ToLower(key)
	if strings.HasSuffix(key, binHdrSuffix) {
		val = base64.StdEncoding.EncodeToString([]byte(val))
	}
	return key, val
}
