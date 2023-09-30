package common

import (
	"context"
	"io"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

type Server interface {
	Hello(name string) (ret string, err error)
}

type TestServer struct {
	host string
}

func (s *TestServer) Hello(name string) (ret string, err error) {
	client := &http.Client{}
	request, err := http.NewRequest("GET", "http://"+s.host+"/ui/dc1/services", nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return "hello " + string(body), nil
}

func MakeTestHelloEndpoint(svc Server) (ep endpoint.Endpoint) {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		name := request.(string)
		ret, err := svc.Hello(name)
		return ret, err
	}
}

func NewTestServer(host string) *TestServer {

	return &TestServer{host: host}
}
