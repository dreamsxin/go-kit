# 教程：生成一个服务

[English](tutorial-microgen.md) | 简体中文

本教程从一个 Go 接口生成完整服务：HTTP 路由、Go 与 TypeScript SDK、OpenAPI，以及
追踪归属关系的清单文件。

## 1. 安装 microgen

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@latest
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
microgen \
  -idl idl.go \
  -out hello-svc \
  -import example.com/hello-svc \
  -protocols http \
  -config

cd hello-svc
go mod tidy
go run ./cmd
```

microgen 会自己创建 `-out` 目录。`-import` 是让生成的代码树可编译的关键——它就是写入
`go.mod` 的模块路径，也是所有生成代码 import 时用的前缀，务必传入。`-config` 生成
配置包，包括下文提到的用户自有 `config/custom.go`。

在第二个终端中：

```bash
cat .microgen/manifest.json
curl http://localhost:8080/health
```

## 4. 理解你拥有什么

清单（`.microgen/manifest.json`）记录来源模式、已启用的能力、路由前缀，以及每一件
生成器拥有的产物。你负责编辑：

- `service/helloservice/service.go` —— 业务逻辑，初始返回 not-implemented 错误；
- `config/custom.go` —— 应用自有的配置扩展（仅当用 `-config` 生成时存在）；
- `cmd/custom_routes.go` —— 生成路由之外的额外路由。

这些文件只写一次、绝不覆盖，`cmd/main.go` 同样如此。其余的一切——endpoint、
transport、SDK、model、repository 以及生成的配置文件——都归生成器所有，每次运行都会
重写，无论文件名是否以 `generated_` 开头。「这个文件我能改吗」的权威答案是清单里的
`artifacts` 列表。

## 5. 添加 OpenAPI 与 TypeScript SDK

```bash
microgen -idl idl.go -out hello-svc -import example.com/hello-svc \
  -protocols http -config -openapi
```

这会添加 `docs/openapi.json`、`docs/schema.json`、位于 `/swagger/` 的内嵌 Swagger
UI，以及 `sdk/typescript/` 下零依赖的 TypeScript Fetch 客户端。Go SDK 不在其中：
`sdk/helloservicesdk/` 在第 3 步就已无条件生成。

在既有项目上重新运行生成，就是添加能力的正常做法。没有 `-force`；用户自有的文件因为
已经存在而被跳过。

## 6. 之后扩展

不重新生成整个项目即可追加服务或模型：

```bash
microgen extend -check -out .
microgen extend -idl full_combined.go -out . -append-model Product
```

文件名 `full_combined.go` 是字面上的建议，不是装饰：`-idl` 必须包含**完整**契约——
所有既有服务与模型，加上新增的那个。否则 extend 会拒绝执行，报
`append-model requires a full combined Go IDL contract; missing existing model definitions for: ...`。

第一次运行前还有三条规则值得知道：

- `-check` 校验清单，且不能与任何 append 标志同时使用；请先单独运行它。
- 每次调用只能追加一项：`-append-service`、`-append-model` 与 `-append-middleware`
  互斥。
- 能力是从清单读回的，不是从标志读的。给 `extend` 传 `-openapi` 或 `-config` 没有
  任何效果；项目保持生成时的能力集合。

extend 会刷新每一件生成器拥有的产物，因此用户自有的文件得以保留。

## 接下来去哪

- [MICROGEN 指南](../MICROGEN_zh.md)：每一个选项、extend 模式与归属
- [教程：一个 CRUD 服务](tutorial-crud_zh.md)：手写出来的同样形态
- [测试](testing_zh.md)：测试生成的服务
