package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
	"github.com/dreamsxin/go-kit/v2/transport/http/interfaces"
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
	applyRequestHeaders(req, request)
	var b bytes.Buffer
	req.Body = io.NopCloser(&b)
	return req, json.NewEncoder(&b).Encode(request)
}

// EncodeQueryRequest encodes request fields into URL path placeholders and
// query parameters. It does not create an HTTP request body.
func EncodeQueryRequest(_ context.Context, req *http.Request, request interface{}) (*http.Request, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	encoded, err := transporthttp.EncodePathAndQuery(req.URL.String(), request)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(encoded)
	if err != nil {
		return nil, fmt.Errorf("parse encoded query URL: %w", err)
	}
	req.URL = parsed
	req.Body = nil
	req.Header.Del("Content-Type")
	applyRequestHeaders(req, request)
	return req, nil
}

func applyRequestHeaders(req *http.Request, request interface{}) {
	if headerer, ok := request.(interfaces.Headerer); ok {
		headers := headerer.Headers()
		for k := range headers {
			req.Header.Set(k, headers.Get(k))
		}
	}
}
