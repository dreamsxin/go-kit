# profilesvc

This example demonstrates how to use Go kit to implement a REST-y HTTP service.
It leverages the excellent [gorilla mux package](https://github.com/gorilla/mux) for routing.

Run the example with the optional port address for the service: 

```bash
$ go run ./cmd/profilesvc/main.go -http.addr :8080
ts=2018-05-01T16:13:12.849086255Z caller=main.go:47 transport=HTTP addr=:8080
```

Create a Profile:

```bash
$ curl -d '{"id":"1234","Name":"Go Kit"}' -H "Content-Type: application/json" -X POST http://localhost:8080/profiles/
{}
```

Get the profile you just created

```bash
$ curl localhost:8080/profiles/1234
{"profile":{"id":"1234","name":"Go Kit"}}
```

## 目录结构

```plainText
examples/profilesvc/
├── README.md          # 使用说明文档
├── client/client.go   # 服务客户端实现（含服务发现与负载均衡）
├── cmd/profilesvc/main.go  # 服务启动入口
├── service.go         # 业务逻辑定义（Profile CRUD接口）
├── endpoints.go       # 端点封装（服务与传输层桥接）
├── transport.go       # HTTP传输层实现
└── middlewares.go     # 中间件（日志）
```
