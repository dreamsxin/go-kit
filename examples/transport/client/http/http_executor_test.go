package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/examples/common"
	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/consul"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	transportclient "github.com/dreamsxin/go-kit/transport/http/client"

	capi "github.com/hashicorp/consul/api"
)

// go test -v -count=1 -run TestExecutorHttpClient .\http_executor_test.go
func TestExecutorHttpClient(t *testing.T) {

	logger, _ := log.NewDevelopment()

	// 连接 consul
	cfg := capi.DefaultConfig()
	client, err := capi.NewClient(cfg)
	if err != nil {
		t.Fatalf("error when creating a new HTTP client: %v", err)
	}

	var header http.Header

	// 模拟http 服务
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		header = r.Header
		body := strings.Trim(string(b), "\n")
		w.Write([]byte(body))
	}))

	defer server.Close()

	if err != nil {
		t.Fatal(err)
	}
	serverURL, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(serverURL.Port())

	// 注册服务
	serverName := "test"
	registrar := consul.NewRegistrar(consul.NewClient(client), logger, serverName, serverURL.Host, port)
	registrar.Register()
	defer registrar.Deregister()

	// 创建端点工厂
	factory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
		logger.Sugar().Debugln("instance", instance)
		// 客户端
		ep := transportclient.NewClient(
			"POST",
			serverURL,                         // 项目中使用端点工厂传回的 instance
			transportclient.EncodeJSONRequest, // 编码
			func(ctx context.Context, res *http.Response) (interface{}, error) {
				body, _ := io.ReadAll(res.Body)
				t.Log("response:", string(body))
				return string(body), nil
			},
		).Endpoint()

		return ep, nil, nil
	}

	// 创建服务发现器
	instrancer := consul.NewInstancer(consul.NewClient(client), logger, serverName, true)

	// 创建端点生成器
	endpointer := endpointer.NewEndpointer(instrancer, factory, logger)

	// 创建负载均衡器
	robin := balancer.NewRoundRobin(endpointer)

	// 创建执行器
	retry := executor.Retry(5, time.Duration(1*time.Second), robin)
	ret, err := retry(context.TODO(), &common.UserData{Foo: "foo"})
	if err != nil {
		//rettyErr := err.(executor.RetryError)
		logger.Sugar().Debugln("-----------TestExecutorRetry--------", ret, err.Error())
	} else {
		logger.Sugar().Debugln("-----------TestExecutorRetry--------", ret)
	}

	if ret != `{"foo":"foo"}` {
		t.Fatalf("ret value: actual %v, expected %v", ret, `{"foo":"foo"}`)
	}

	if _, ok := header["X-Email"]; !ok {
		t.Fatalf("X-Email value: actual %v, expected %v", nil, []string{"dreamsxin@qq.com"})
	}

	if v := header.Get("X-Email"); v != "dreamsxin@qq.com" {
		t.Errorf("X-Email string: actual %v, expected %v", v, "dreamsxin@qq.com")
	}
}
