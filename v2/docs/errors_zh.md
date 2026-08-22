# 错误处理

[English](errors.md) | 简体中文

go-kit v2 将错误分类（业务）与错误编码（传输层）分离。本页覆盖整个流程：分类、
自动状态码映射、校验错误与自定义线上格式。

## 业务错误分类

`apperror` 在不引入任何传输层类型的情况下对失败进行分类：

```go
import "github.com/dreamsxin/go-kit/v2/apperror"

func (s todoService) Get(ctx context.Context, id int64) (Todo, error) {
	if id <= 0 {
		return Todo{}, apperror.New(
			apperror.KindInvalidArgument,
			"todo.invalid_id",
			"todo id must be a positive integer",
		)
	}
	todo, err := s.store.get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, apperror.New(
			apperror.KindNotFound,
			"todo.not_found",
			"todo not found",
		)
	}
	return todo, err
}
```

每个 `apperror.Error` 都携带一个稳定的机器可读 `code` 和一个公开 `message`；两者
都可以安全地暴露给客户端。未分类的错误返回 500，且不泄露内部细节。

### `apperror` 参考

包路径为 `github.com/dreamsxin/go-kit/v2/apperror`，属于核心模块，无额外
依赖：

| API | 用途 |
| --- | --- |
| `New(kind, code, message)` | 创建带稳定 code 与公开 message 的分类错误 |
| `Wrap(kind, code, message, cause)` | 同上，并保留底层原因供 `errors.Is/As` 使用 |
| `Error.ErrorKind()` | 传输无关的错误种类 `Kind` |
| `Error.ErrorCode()` | 稳定的机器可读 code |
| `Error.PublicMessage()` | 可安全展示给客户端的 message |

错误种类与 HTTP 状态码映射：

| Kind | HTTP 状态码 |
| --- | --- |
| `KindInvalidArgument` | 400 |
| `KindUnauthenticated` | 401 |
| `KindPermissionDenied` | 403 |
| `KindNotFound` | 404 |
| `KindAlreadyExists`、`KindConflict` | 409 |
| `KindFailedPrecondition` | 412 |
| `KindResourceExhausted` | 429 |
| `KindUnavailable` | 503 |
| `KindDeadlineExceeded` | 504 |
| `KindInternal` | 500 |

## 自动状态码映射

传输层把 kind 映射为状态码：HTTP 400/401/403/404/409/429/500 以及对应的 gRPC 码。
参见[核心概念](concepts_zh.md)中的表格。

默认的 JSON 错误体为：

```json
{"code": "todo.not_found", "message": "todo not found", "request_id": "..."}
```

## 校验错误

实现了 `endpoint.Validatable` 的请求会在业务逻辑运行之前被校验；字段失败会收集到
`endpoint.ValidationError` 中，它以稳定码 `bad_request.validation` 编码为 400：

```go
func (r CreateUserRequest) Validate() error {
	if r.Name == "" {
		return endpoint.NewValidationError("name", "is required")
	}
	return nil
}

ep := endpoint.NewBuilder(createUser).WithValidation().Build()
```

## 自定义错误格式

JSON 入口点允许按路由传入自定义错误编码器；使用 `kit` 时，在装配处为所有路由
安装一个：

```go
kit.New(":8080", kit.WithJSONServerOptions(
	server.ServerErrorEncoder(myErrorEncoder),
))
```

编码器接收错误和 `http.ResponseWriter`。一种典型的自定义格式保留分类，但改变信封：

```go
func myErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "internal error"})
		return
	}
	// 复用框架的分类逻辑，不要重复实现映射。
	status := server.HTTPStatusForError(err)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": status, "message": appErr.PublicMessage()})
}
```

可运行的演练见 [examples/envelope](../examples/README_zh.md)。

## 经验法则

- 业务代码用 `apperror` 分类；传输层映射状态码；绝不向客户端泄露内部细节。
- 客户端错误（4xx）携带公开消息；服务器错误（5xx）保持不透明。
- 重试策略属于调用方：只有已分类的、幂等的失败才应该重试。
