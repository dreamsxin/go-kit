# 快速上手

[English](getting-started.md) | 简体中文

本页带你在大约五分钟内，从空目录走到一个能应答 HTTP 请求的运行中服务。你只需要
Go 1.25.8 或更高版本。

## 安装

```bash
go get github.com/dreamsxin/go-kit/v2@v2.6.0
```

## 第一个服务

创建 `main.go`：

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamsxin/go-kit/v2/kit"
)

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Message string `json:"message"`
}

func main() {
	svc, err := kit.NewHTTP(":8080", kit.WithRequestID())
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSONTyped(svc, "POST /greet", func(
		_ context.Context, req GreetRequest,
	) (GreetResponse, error) {
		return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
	})

	host, err := kit.NewHost(kit.WithLifecycle(svc))
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

运行它，然后在第二个终端中调用：

```bash
go run .
curl -X POST http://localhost:8080/greet \
  -H "Content-Type: application/json" \
  -d '{"name":"world"}'
curl http://localhost:8080/health
```

服务会应答 `{"message":"Hello, world!"}`，而 `/health` 会免费报告进程状态。

## 刚才发生了什么

- `kit.NewHTTP` 校验了配置并注册了 `/health`、`/livez` 和 `/readyz`。
- `HandleJSONTyped` 注册了一个类型化 JSON 路由：请求体被严格解码（未知字段和
  多余数据都会被拒绝），响应以匹配的状态码编码为 JSON。
- `kit.NewHost` 把 HTTP 组件挂载到传输中立的生命周期 Host；`host.Run(ctx)`
  启动了服务器，并在收到 `SIGTERM` 时将其优雅停机。

## 下一步

- 与其手写，不如生成一个完整项目（客户端、SDK、OpenAPI）：
  [教程：生成一个服务](tutorial-microgen_zh.md)
- 添加存储与 CRUD：[教程：一个 CRUD 服务](tutorial-crud_zh.md)
- 了解请求路径：[核心概念](concepts_zh.md)
- 浏览每一个可运行示例：[examples](../examples/README_zh.md)
