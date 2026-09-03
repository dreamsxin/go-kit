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
| `KindCanceled` | 499 | Canceled |
| `KindInternal` | 500 | Internal |
| `KindUnimplemented` | 501 | Unimplemented |
| `KindUnavailable` | 503 | Unavailable |
| `KindDeadlineExceeded` | 504 | DeadlineExceeded |
| 未分类 | 500 | Internal（对客户端不透明） |

499 是 nginx 的非标准状态码 "Client Closed Request"，gRPC-gateway 也用它。
常量为 `server.StatusClientClosedRequest`。用它可以把客户端断连排除在 5xx
错误率之外。

endpoint 层的拒绝错误自带分类，因此两种传输从它们得到一致的语义：

| 拒绝错误 | Kind | HTTP 状态码 |
| --- | --- | --- |
| `ErrRateLimited` | `KindResourceExhausted` | 429 |
| `ErrCircuitOpen` | `KindUnavailable` | 503 |
| `ErrBulkheadFull` | `KindUnavailable` | 503 |
| `ErrBackpressure` | `KindUnavailable` | 503 |

限流是调用方超出了自己的配额（429）；熔断打开或过载卸载说明服务或其依赖不可用
（503）。Envoy 与 Resilience4j 做的是同样的区分。

未分类的 context 错误按对应 kind 映射，这样超时和断连无论是否显式分类都表现
一致：

- `context.DeadlineExceeded`——即 `TimeoutMiddleware` 暴露出来的错误——编码为
  504，与 `KindDeadlineExceeded` 一致。
- `context.Canceled` 编码为 499，与 `KindCanceled` 一致。

显式分类优先：包裹了 context 错误的 `apperror` 仍按自己的 kind 映射。

### 重试提示

错误可以通过实现 `endpoint.RetryAfterReporter` 告知客户端需要等待多久。
`endpoint.NewRetryAfterError(err, delay)` 在不改变错误身份的前提下附加该提示，
`errors.Is` 与分类信息都保持可用。

HTTP 编码器会把它写成 `Retry-After` 头（单位秒，向上取整）；gRPC 编码器会在
status 上附加 `google.rpc.RetryInfo`。熔断器在开窗期内拒绝时会自动上报开窗剩余
时间；半开状态下的拒绝（已有另一个探测请求在途）不带提示，因为开窗已经结束，
没有可诚实上报的时间——调用方回退到自己的退避策略。实现了 `RetryAfterReporter`
的 `RateLimiter` 的延迟会被附加到 `ErrRateLimited` 上。

`RetryMiddleware` 反向读取同一契约：错误带提示时，该提示取代本地退避计划，并以
`endpoint.MaxRetryAfterHint` 封顶，避免恶意对端把没有自带 deadline 的调用方长
时间挂住。

### 客户端分类

客户端调用本身就是 endpoint，因此同一套中间件必须能理解它的错误。
`client.HTTPStatusError` 自己完成分类：通过把响应状态码反查为 kind
（`client.KindForStatus`）实现 `apperror.Kinder`，通过解析 `Retry-After` 响应头
（同时支持秒数与 HTTP-date 两种形式）实现 `RetryAfterReporter`。由此 `WithRetry`
无需自定义分类器即可用于客户端 endpoint：

```go
call, _ := client.NewJSONClient[UserResp](http.MethodGet, "https://api/users/1")
ep := endpoint.NewBuilder(call).
    WithTimeout(2 * time.Second).
    WithCircuitBreaker(breaker).
    WithRetry(3). // 重试 408/429/5xx，不重试其他 4xx，并遵循服务端的 Retry-After
    Build()
```

把上游错误返回给自己的调用方之前要显式转译。原样透传上游的 `KindNotFound` 会让
你的 handler 因为依赖缺记录而回 404：

```go
if _, err := call(ctx, req); err != nil {
    return apperror.WrapCause(apperror.KindUnavailable, "upstream.users", err)
}
```

gRPC 客户端是对称的：失败调用会被包装为 `grpc.StatusError`，它把状态码映射为 kind
（`grpc.KindNameForCode`）并上报 `google.rpc.RetryInfo` 中的延迟，同时
`status.FromError` 照常可用。

`DefaultRetryable` 同时识别可选契约 `interface{ Retryable() bool }`，让自定义客户端
错误自行决定。优先级为：context 错误与本地准入拒绝永不重试，其次 `Retryable()`，
最后 `apperror.KindUnavailable`。

## 标识失败的是稳定业务码，不是状态码

状态码是粗粒度信道——只有几十个取值，承载不了业务语义，也无法演进。这份工作由稳定
业务码承担，因此每种传输都在状态码之外单独携带它：

- HTTP：放在消息体里，形如 `{"code": "user.missing", ...}`。
- gRPC：按 AIP-193 放在 `google.rpc.ErrorInfo` detail 的 `reason` 字段。
  `NewErrorEncoder(WithErrorDomain("users.example.com"))` 设置这些码所属的 `domain`。

两侧客户端都会把它读回来，于是转发时业务码不会退化成按状态码生成的名字：

- `client.HTTPStatusError.ErrorCode()` 解析 JSON 消息体。
- `grpc.StatusError.ErrorCode()` 读取 `ErrorInfo` 的 reason，`ErrorDomain()` 读取其
  domain。

由于 `HTTPStatusError` 与 `StatusError` 都满足 `transport/http.ErrorCoder`，原样转发
会复现上游的业务码：

```go
// 上游返回 404 {"code":"user.missing"}
// 本服务同样返回 404 {"code":"user.missing"}
```

上游的 *message* 有意不作为可公开消息转发：它属于另一个服务的契约。要对外说明请用
`PublicMessage` 写自己的。

## 自动状态码映射

传输层按上表把 kind 映射为状态码；业务代码不接触任何协议类型。
`server.HTTPStatusForError` 暴露完整规则（含 `StatusCoder` 与未分类的 context
错误），自定义编码器应复用它而不是重复实现。

默认的 JSON 错误体为：

```json
{"code": "todo.not_found", "message": "todo not found", "request_id": "..."}
```

`message` 的取值顺序三个内置编码器共用：`PublicMessager` 优先；否则低于 500 的状态码
可以用 `err.Error()`；500 一律回答 `"Internal Server Error"`。500 正是未分类错误的落点，
而未分类错误恰好就是那种文本从来不是写给客户端看的错误——它可能包着驱动报错或上游
响应体。错误本身仍然完整进日志，`request_id` 把两边串起来。通过 `StatusCoder` 主动
设置的 5xx 保留自己的消息。

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

映射器解析的是 *kind*，不是状态码：通过 `transporthttp.StatusCoder` 声明了自身状态
码的错误会保留它，与默认编码器一致。正是这一点保证了映射器不会静默改写转发的
`client.HTTPStatusError` 或 `endpoint.ValidationError`。

## 经验法则

- 业务代码用 `apperror` 分类；传输层映射状态码；绝不向客户端泄露内部细节。
- 客户端错误（4xx）携带公开消息。500 永远不带——它是未分类失败的落点。主动分类的
  501/503/504 如果错误通过 `PublicMessage` 声明了消息就会带上；`err.Error()` 在 5xx
  下从不使用。
- 重试策略属于调用方：只有已分类的、幂等的失败才应该重试。
