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
| `WrapCause(kind, code, cause)` | 同上但不带公开 message，用于原因必须留在内部的场景 |
| `Error.ErrorKind()` | 传输无关的错误种类 `Kind` |
| `Error.ErrorCode()` | 稳定的机器可读 code |
| `Error.PublicMessage()` | 可安全展示给客户端的 message |

每个 kind 还有同名构造函数——`NotFound(code, message)`、
`DeadlineExceeded(code, message)` 等——用于缩短调用点。

错误种类与传输层映射。本表是内置映射的唯一来源，传输层实现分别是
`server.HTTPStatusForErrorKind` 与 `grpcserver.CodeForErrorKind`：

| Kind | HTTP 状态码 | gRPC 码 |
| --- | --- | --- |
| `KindInvalidArgument` | 400 | InvalidArgument |
| `KindUnauthenticated` | 401 | Unauthenticated |
| `KindPermissionDenied` | 403 | PermissionDenied |
| `KindNotFound` | 404 | NotFound |
| `KindAlreadyExists` | 409 | AlreadyExists |
| `KindConflict` | 409 | Aborted |
| `KindFailedPrecondition` | 412 | FailedPrecondition |
| `KindResourceExhausted` | 429 | ResourceExhausted |
| `KindInternal` | 500 | Internal |
| `KindUnavailable` | 503 | Unavailable |
| `KindDeadlineExceeded` | 504 | DeadlineExceeded |
| 未分类 | 500 | Internal（对客户端不透明） |

有两组错误不经分类也会被映射：

- endpoint 层的拒绝错误（`ErrRateLimited`、`ErrCircuitOpen`、
  `ErrBulkheadFull`、`ErrBackpressure`）编码为 429。
- 未分类的 `context.DeadlineExceeded`——也就是端点超时时
  `TimeoutMiddleware` 暴露出来的错误——编码为 504，与 `KindDeadlineExceeded`
  一致。显式分类优先：包裹了超时原因的 `apperror` 仍按自己的 kind 映射。

## 自动状态码映射

传输层按上表把 kind 映射为状态码；业务代码不接触任何协议类型。
`server.HTTPStatusForError` 暴露完整规则（含 `StatusCoder`、拒绝错误与
context 超时），自定义编码器应复用它而不是重复实现。

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
kit.NewHTTP(":8080", kit.WithJSONServerOptions(
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

## 自定义错误种类与状态码

应用可以定义自己的 `apperror.Kind` 值（kind 是字符串）。用
`JSONErrorEncoderWithKindMapper` 一次性注册状态码映射；未知 kind 回退到
内置映射：

```go
svc, err := kit.NewHTTP(":8080", kit.WithJSONServerOptions(
	server.ServerErrorEncoder(server.JSONErrorEncoderWithKindMapper(func(k apperror.Kind) int {
		if k == "payment_failed" {
			return http.StatusPaymentRequired
		}
		return 0 // 回退到内置映射
	})),
))
```

自定义编码器需要与内置映射组合而不是替换时，`server.HTTPStatusForErrorKind(kind)`
公开内置映射。

## 经验法则

- 业务代码用 `apperror` 分类；传输层映射状态码；绝不向客户端泄露内部细节。
- 客户端错误（4xx）携带公开消息；服务器错误（5xx）保持不透明。
- 重试策略属于调用方：只有已分类的、幂等的失败才应该重试。
