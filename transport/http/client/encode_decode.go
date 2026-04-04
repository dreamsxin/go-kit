package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dreamsxin/go-kit/transport/http/interfaces"
)

// EncodeRequestFunc encodes a domain request value into an *http.Request.
// The req parameter may be nil; implementations should create a new request
// in that case.
type EncodeRequestFunc func(context.Context, *http.Request, interface{}) (*http.Request, error)

// DecodeResponseFunc decodes an *http.Response into a domain response value.
type DecodeResponseFunc func(context.Context, *http.Response) (response interface{}, err error)

// EncodeJSONRequest JSON-encodes the request body and sets Content-Type to
// application/json.  If the request implements interfaces.Headerer, those
// headers are also added.
func EncodeJSONRequest(c context.Context, req *http.Request, request interface{}) (*http.Request, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if headerer, ok := request.(interfaces.Headerer); ok {
		for k := range headerer.Headers() {
			req.Header.Set(k, headerer.Headers().Get(k))
		}
	}
	var b bytes.Buffer
	req.Body = io.NopCloser(&b)
	return req, json.NewEncoder(&b).Encode(request)
}
