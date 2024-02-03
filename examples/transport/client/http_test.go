package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	transportclient "github.com/dreamsxin/go-kit/transport/http/client"
)

type UserData struct {
	Foo string `json:"foo"`
}

func (e UserData) Headers() http.Header {
	return http.Header{"X-Email": []string{"dreamsxin@qq.com"}}
}

// go test -v -count=1 -run TestHttpClient .\http_test.go
func TestHttpClient(t *testing.T) {
	var header http.Header
	var body string

	// 模拟http 服务
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		header = r.Header
		body = strings.Trim(string(b), "\n")
		w.Write([]byte(body))
	}))

	defer server.Close()

	serverURL, err := url.Parse(server.URL)

	if err != nil {
		t.Fatal(err)
	}

	// 客户端
	client := transportclient.NewClient(
		"POST",
		serverURL,                         // 项目中使用端点工厂传回的 instance
		transportclient.EncodeJSONRequest, // 编码
		func(context.Context, *http.Response) (interface{}, error) {
			t.Log("response:", body)
			return nil, nil
		},
	).Endpoint()

	if _, err := client(context.Background(), &UserData{Foo: "foo"}); err != nil {
		t.Fatal(err)
	}

	if body != `{"foo":"foo"}` {
		t.Fatalf("body value: actual %v, expected %v", body, `{"foo":"foo"}`)
	}

	if _, ok := header["X-Email"]; !ok {
		t.Fatalf("X-Email value: actual %v, expected %v", nil, []string{"dreamsxin@qq.com"})
	}

	if v := header.Get("X-Email"); v != "dreamsxin@qq.com" {
		t.Errorf("X-Email string: actual %v, expected %v", v, "dreamsxin@qq.com")
	}
}
