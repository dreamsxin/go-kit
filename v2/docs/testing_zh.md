# 测试

[English](testing.md) | 简体中文

业务逻辑就是一个 `(context, Request) -> (Response, error)` 的普通函数，因此大多数
测试完全不需要服务器。HTTP 行为用 `httptest.NewServer` 覆盖，它直接接受一个
`kit.Service`。

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

`kit.Service` 实现了 `http.Handler`，因此 `httptest.NewServer` 无需占用端口即可
为其提供服务：

```go
func TestHTTP_Greet(t *testing.T) {
	svc := kit.MustNew(":0", kit.WithRequestID())
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

`MustNew` 是测试专用构造函数；它在配置非法时 panic，这正是测试想要的。

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

## 参考模式

示例测试是权威参考：`examples/quickstart`、`examples/todosvc`（服务层、存储层与
HTTP 层）和 `examples/auth`（中间件与状态码）各自演示了请求路径中的一层。
