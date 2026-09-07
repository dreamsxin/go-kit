# 指标 exposition
[English](README.md) | 简体中文

`observability/metrics` 把 `endpoint.Metrics` 已经收集到的东西渲染成 Prometheus 文本
exposition。它只用标准库——依赖门禁把任何指标客户端挡在外面，所以挂一个 scrape 端点不会给构建
增加任何东西。

```go
collector := &endpoint.Metrics{}

var routes httpserver.RouteRegistrar = mux
routes = httpserver.DecorateRoutes(mux,
    httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector)))
routes.Handle("GET /users/{id}", usersHandler)

mux.Handle("GET /metrics", metrics.Handler(collector))
```

recording 安装在注册处，而不是包在 mux 外面：`http.Request.Pattern` 只在 mux 分派出去的那个
请求上有值，所以包在 mux 外面的中间件会把每个请求都记到空路由下。exposition 挂在 mux 本身、在
装饰器之外，于是 scrape 报告的是服务，而不是它自己。

一次 scrape 会返回什么：每个 operation 的请求数、错误数、耗时总和与最后一次请求的时间戳，其中
operation 是匹配到的路由 pattern。那是均值，不是分位数——收集器持有的是总量，没有 bucket 可以
推出 p99。需要分位数时用 `observability/otel` 里的 OpenTelemetry 直方图。计数器在进程启动时
从零开始，采集方会把这读成一次 reset。

gRPC 的桥接在 `observability/metrics/grpc`，独立成包，正是为了让这个包仍然只靠标准库就能用。
它用完整方法名给每条序列打标签，而方法集合由服务定义限定。

挂不挂由应用决定：`kit` 不导入这个包，依赖门禁也保证它不会——所以路由、它挂在哪个监听器上、
以及要不要存在，都留给部署方。生成服务提供 `server.metrics_path`（`APP_METRICS_PATH`），默认为
空即关闭——该端点会公开路由名与流量形状，所以把它放在 admin 监听或网络策略之后，而不是放在对用户
提供服务的那个端口上。
