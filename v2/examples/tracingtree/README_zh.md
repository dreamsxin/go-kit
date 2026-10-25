# tracingtree

[English](README.md) | 简体中文

一次请求穿过完整的可观测栈，打印成一棵树：

```text
trace 5ca10c5f1068fcf96b2a7d46848656e3
  └─ server GET /aggregate
    └─ internal GET /aggregate
      └─ client GET
```

三个层级就是框架的分工，被摆到了眼前：

- `server GET /aggregate` —— `oteladapter.HTTPServerMiddleware` 包住整个
  服务器，每个请求一个 SERVER span，以 mux 命中的路由命名。
- `internal GET /aggregate` —— `oteladapter.TracingMiddleware` 包住
  endpoint，业务代码与其下的每个中间件都汇入这个 span。
- `client GET` —— `oteladapter.NewRoundTripper` 包住出站传输，同时注入
  trace context；若上游也接了追踪，同一棵树会在那里继续。

运行：

```bash
go run ./examples/tracingtree
```

打印树的导出器是刻意手写的：provider、导出器及其关闭归应用所有，几十行是能
看清形状的最小导出器。部署时把它换成指向 collector 的 `oteladapter.Setup`
即可——见[可观测性一章](../../docs/observability_zh.md)。
