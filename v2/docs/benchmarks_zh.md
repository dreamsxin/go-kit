# 基准

[English](benchmarks.md) | 简体中文

两个 harness 回答两个不同的问题，哪个答案都不是营销数字：框架在它所坐的平台之上
花掉了什么，以及这份开销与 Go 服务真实会在其中选择的框架相比是什么水平。两者都能
从本仓库复现；环境一节不是可选背景，它是结果的一部分。

## 方法论

一条 JSON POST 路由（入 `{"name","count"}`，出 `{"echo","double"}`），每个参比者
各起一个服务（同一测试进程内），一条 keep-alive 连接按服务器应答速度打满，丢弃
响应体。`go test -bench`
报告 ns/op、B/op 与 allocs/op；下面的数字是 `-benchtime=2s` 的运行。没有 TLS、
没有超过一段话的请求体、没有连接抖动——那些量的是传输层和客户端，不是框架，而且
选哪个框架也改变不了它们。

## 框架在标准库之上花掉什么

`examples/benchmark`，同一进程内运行：

```bash
go -C v2/examples test ./benchmark -bench . -benchmem -benchtime=2s
```

| 路由 | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `net/http` + `encoding/json` | 87,501 | 8,525 | 94 |
| kit（`kit.NewHTTP` + `kit.HandleJSON`） | 97,561 | 9,762 | 117 |

框架的 endpoint 中间件链、严格 JSON 解码、校验钩子与错误分类，相比裸平台每请求
约 11% 延迟、23 次分配。这条路由带着框架的校验 tag，所以这个数字包含裸 handler
不会做的工作。

## 与其他框架对比

`tools/testdata/bench-thirdparty`——刻意是独立 module，因为对比 harness 不该把
四个框架塞进发布物的依赖图：

```bash
cd v2/tools/testdata/bench-thirdparty
GOWORK=off go test -bench . -benchmem -benchtime=2s
```

同一条路由、同一台机器（Intel i5-10400F、Windows、loopback、go1.26.0）：

| 框架 | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| stdlib `ServeMux` | 88,598 | 9,095 | 95 |
| **kit** | **97,881** | **10,215** | **117** |
| chi | 92,384 | 9,599 | 97 |
| gin | 93,148 | 9,341 | 95 |
| echo | 91,719 | 9,396 | 95 |

读法：这条路由上 kit 比路由器框架贵 5–7%，五者都在平台的 ~11% 之内。Windows
loopback 上传输层占主导；绝对数字随机器漂移，可复现的是排序。任何承诺比这更精确
的框架基准都是营销，包括我们的。

## 这些数字没说的

单请求吞吐不是服务选框架的理由，否则标准库每次都赢。上面这些框架真正的差别在
路由周围的东西：严格解码、校验、错误分类与信封、OpenAPI 生成、追踪与指标接线、
服务发现、带反馈的重试、以及 MCP 支持。[功能章节](index_zh.md)才是诚实的对比；
这一页的存在是为了让那些功能的价格也摆在桌面上。
