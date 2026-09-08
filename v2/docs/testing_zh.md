# 测试

[English](testing.md) | 简体中文

业务逻辑就是一个 `(context, Request) -> (Response, error)` 的普通函数，因此大多数
测试完全不需要服务器。选择能够证明行为的最小测试边界。

## 按改动选择测试

| 改了什么 | 先运行 |
| --- | --- |
| service 规则或错误 kind | 直接调用 service |
| endpoint 中间件 | 直接调用构建后的端点 |
| HTTP 解码、状态码、header | 用 `kit.HTTP` 配合 `httptest.NewServer` |
| 生成项目或 SDK 契约 | `go test ./tools -run 'TestMicrogen'` |
| 并发或生命周期 | 针对包运行 `go test -race` |

完整生成项目和进程 smoke 测试放在集成测试或发布验证中；它们有意更慢。

## 单元测试业务逻辑

```go
func TestGreet_EmptyName(t *testing.T) {
	_, err := greet(context.Background(), GreetRequest{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}
```

已分类的错误通过 `apperror` 断言：

```go
var appErr *apperror.Error
if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindInvalidArgument {
	t.Fatalf("expected invalid_argument, got %v", err)
}
```

## 测试 HTTP 表面

`kit.HTTP` 实现了 `http.Handler`，因此 `httptest.NewServer` 无需占用端口即可
为其提供服务：

```go
func TestHTTP_Greet(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithRequestID())
	kit.HandleJSONTyped(svc, "/greet", greet)

	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/greet", "application/json",
		strings.NewReader(`{"name":"kit"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// assert status, body, and headers
}
```

`MustNewHTTP` 是测试专用构造函数；它在配置非法时 panic，这正是测试想要的。

## 测试中间件链

构建端点并直接调用它：

```go
ep := endpoint.NewBuilder(base).WithValidation().Build()
if _, err := ep(context.Background(), invalidRequest); err == nil {
	t.Fatal("validation should reject the request")
}
```

拒绝错误用 `errors.Is` 断言：

```go
if !errors.Is(err, endpoint.ErrRateLimited) {
	t.Fatalf("expected rate limit rejection, got %v", err)
}
```

## 集成测试

集成测试启动的是部署会启动的那个东西。有两个边界值得为它付出代价。

生成的项目本来就带了一份脚手架。`microgen -tests` 会为每个服务写一个
`test/<service>_test.go`，它构造服务，并用零值请求调用每个方法：一次直接调用，一次
经过 `LoggingMiddleware`。它证明接线能编译、能返回；它对你的业务规则什么都不断言。
那个文件归生成器所有，重新生成时会被覆盖，所以真正的断言要放在你自己的文件里：

```bash
go test ./...          # 在生成项目的根目录执行
```

再往上一层，是运行编译出来的二进制并通过网络驱动它。要等待就绪，而不是 sleep：
`/readyz` 在两个监听都开始服务之前返回 503，在关闭一开始时再次返回 503，所以轮询它
才是诚实的闸门。`/health` 行为相同；`/livez` 只要进程活着就返回 200，对就绪状态什么
都不说。没有 `/healthz` 这个路径。

生命周期与并发方面的断言需要竞态检测器，而不是更长的测试：

```bash
go test -race -run 'TestShutdown|TestDrain' -count=1 ./...
```

## 参考模式

示例测试是权威参考：`examples/quickstart`、`examples/todosvc`（服务层、存储层与
HTTP 层）和 `examples/auth`（中间件与状态码）各自演示了请求路径中的一层。
