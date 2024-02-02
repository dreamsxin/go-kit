package basic

import (
	"testing"

	"github.com/dreamsxin/go-kit/sd/consul"

	capi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// go test -v -count=1 -run TestRegistrar .\registrar_test.go
func TestRegistrar(t *testing.T) {
	// Get a new client
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewDevelopment()

	registrar := consul.NewRegistrar(consul.NewClient(client), logger.Sugar(), "test", "localhost", 8500)
	registrar.Register()
	//defer registrar.Deregister()
	//time.Sleep(30 * time.Second)
}

// go test -v -count=1 -run TestDeregister .\registrar_test.go
func TestDeregister(t *testing.T) {
	// Get a new client
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewDevelopment()

	registrar := consul.NewRegistrar(consul.NewClient(client), logger.Sugar(), "test", "localhost", 8500)
	//删除旧的服务实例
	registrar.Deregister()
}

// go test -v -count=1 -run TestRegistrarCheck .\registrar_test.go
func TestRegistrarCheck(t *testing.T) {
	// Get a new client
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewDevelopment()

	registrar := consul.NewRegistrar(consul.NewClient(client), logger.Sugar(), "news", "localhost", 7888, consul.CheckRegistrarOptions(&capi.AgentServiceCheck{
		CheckID:                        "status",
		HTTP:                           "http://localhost:7888/",
		Timeout:                        "5s",  //超时
		Interval:                       "5s",  //健康检查频率
		DeregisterCriticalServiceAfter: "10s", // 故障检查失败30s后 consul自动将注册服务删除
	}))
	registrar.Register()
	//defer registrar.Deregister()
	//time.Sleep(30 * time.Second)
}
