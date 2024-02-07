package client

import (
	"context"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc/metadata"
)

type RequestFunc func(context.Context, *metadata.MD) context.Context

// 自定义 gRPC metadata headers
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

func EncodeKeyValue(key, val string) (string, string) {
	key = strings.ToLower(key)
	if strings.HasSuffix(key, binHdrSuffix) {
		val = base64.StdEncoding.EncodeToString([]byte(val))
	}
	return key, val
}
