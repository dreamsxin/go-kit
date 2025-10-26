# Service Discovery 服务发现模块

服务发现模块提供了微服务架构中的服务注册与发现功能，支持 Consul 等服务发现工具。

## 核心组件

### Consul 集成

- **consul/** - Consul 服务发现集成
  - `client.go` - Consul 客户端封装
  - `instancer.go` - 服务实例发现
  - `registrar.go` - 服务注册管理

### 端点管理

- **endpointer/** - 端点管理器
  - `endpointer.go` - 端点管理核心
  - `balancer/` - 负载均衡器
    - `robin.go` - 轮询负载均衡
  - `executor/` - 执行器
    - `retry.go` - 重试机制

### 实例管理

- **instance/** - 服务实例管理
  - `cache.go` - 实例缓存
  - `registry.go` - 实例注册表

### 事件系统

- **events/** - 事件系统
  - `event.go` - 事件定义和处理

### 接口定义

- **interfaces/** - 接口定义
  - `balancer.go` - 负载均衡器接口
  - `instancer.go` - 实例发现接口
  - `registrar.go` - 服务注册接口

## 快速使用

### 服务注册示例

```go
import (
    "github.com/dreamsxin/go-kit/sd/consul"
    "github.com/hashicorp/consul/api"
)

// 创建Consul客户端
consulClient, _ := api.NewClient(api.DefaultConfig())

// 创建服务注册器
registrar := consul.NewRegistrar(
    consulClient,
    &api.AgentServiceRegistration{
        ID:   "my-service-1",
        Name: "my-service",
        Port: 8080,
    },
    log.NewNopLogger(),
)

// 注册服务
registrar.Register()
defer registrar.Deregister()
```

### 服务发现示例

```go
import "github.com/dreamsxin/go-kit/sd/consul"

// 创建服务发现器
instancer := consul.NewInstancer(
    consulClient,
    log.NewNopLogger(),
    "my-service",
    []string{}, // tags
    true,       // passing only
)

// 创建端点工厂
factory := func(instance string) (endpoint.Endpoint, error) {
    return myEndpointFactory(instance), nil
}

// 创建端点管理器
endpointer := sd.NewEndpointer(instancer, factory, log.NewNopLogger())
```

## API 参考

### 服务注册接口

```go
type Registrar interface {
    Register()
    Deregister()
}
```

### 服务发现接口

```go
type Instancer interface {
    Register(chan<- sd.Event)
    Deregister(chan<- sd.Event)
    Stop()
}
```

### 负载均衡接口

```go
type Balancer interface {
    Endpoint() (endpoint.Endpoint, error)
}
```

## 最佳实践

1. **健康检查**：实现完善的服务健康检查机制
2. **服务标签**：使用标签进行服务分类和路由
3. **缓存策略**：合理配置服务实例缓存时间
4. **重试机制**：实现客户端重试和故障转移
5. **监控告警**：监控服务发现状态和性能指标
