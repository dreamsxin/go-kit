# 教程：生成一个服务

[English](tutorial-microgen.md) | 简体中文

本教程从一个 Go 接口生成完整服务：HTTP 路由、Go 与 TypeScript SDK、OpenAPI，以及
追踪归属关系的清单文件。

## 1. 安装 microgen

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.4
```

## 2. 编写契约

`idl.go` 是一个普通 Go 文件：只有接口与请求/响应类型，不导入框架：

```go
package hello

import "context"

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

type HelloService interface {
	SayHello(context.Context, HelloRequest) (HelloResponse, error)
}
```

## 3. 生成

```bash
mkdir hello-svc
microgen \
  -idl idl.go \
  -out hello-svc \
  -import example.com/hello-svc \
  -protocols http

cd hello-svc
go mod tidy
go run ./cmd
```

在第二个终端中：

```bash
cat .microgen/manifest.json
curl http://localhost:8080/health
```

## 4. 理解你拥有什么

清单（`.microgen/manifest.json`）记录来源模式、已启用的能力、路由前缀，以及每一件
生成器拥有的产物。你负责编辑：

- `service/helloservice/service.go` —— 业务逻辑，初始返回 not-implemented 错误；
- `config/custom.go` —— 应用自有的配置扩展；
- `cmd/custom_routes.go` —— 生成路由之外的额外路由。

所有名为 `generated_*` 的文件都会被重新生成；绝不要手工编辑它们。

## 5. 添加 OpenAPI 与 SDK

```bash
microgen -idl idl.go -out hello-svc -import example.com/hello-svc \
  -protocols http -openapi
```

这会添加 `docs/openapi.json`、`docs/schema.json`、位于 `/swagger/` 的内嵌 Swagger
UI、Go SDK，以及 `sdk/typescript/` 下零依赖的 TypeScript Fetch 客户端。

## 6. 之后扩展

不重新生成整个项目即可追加服务或模型：

```bash
microgen extend -check -out .
microgen extend -idl full_combined.go -out . -append-model Product
```

extend 模式先校验清单，再刷新每一件生成器拥有的产物，因此用户自有的文件得以
保留。

## 接下来去哪

- [MICROGEN 指南](../MICROGEN_zh.md)：每一个选项、extend 模式与归属
- [教程：一个 CRUD 服务](tutorial-crud_zh.md)：手写出来的同样形态
- [测试](testing_zh.md)：测试生成的服务
