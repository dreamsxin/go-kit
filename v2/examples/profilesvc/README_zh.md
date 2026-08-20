# profilesvc（用户资料服务）

[English](README.md) | 简体中文

一个完整的 REST 风格示例，展示了 service、endpoint、中间件、HTTP 传输层，以及一个感知服务发现的客户端。

## 运行

在 v2 模块下：

```bash
go run ./examples/profilesvc/cmd/profilesvc -http.addr=:8080
```

创建并读取一个 profile：

```bash
curl -X POST http://localhost:8080/profiles/ \
  -H "Content-Type: application/json" \
  -d '{"id":"1234","name":"Go Kit"}'

curl http://localhost:8080/profiles/1234
```

## 目录结构

```text
profilesvc/
|-- service.go              business interface and implementation
|-- endpoints.go            endpoint adapters
|-- middlewares.go          service middleware
|-- transport.go            HTTP transport
|-- client/client.go        discovery-aware client
`-- cmd/profilesvc/main.go  process assembly
```

本示例演示的是手动组件装配。如果要创建一个新的生成式服务，请从 `microgen` 开始；如果需要更小规模的装配，请参见 `examples/quickstart`。
