# 校验

[English](validation.md) | 简体中文

框架内有两套校验系统，回答的是不同的问题。本章说明各自适用场景、在线上的
错误形态，以及共享的词汇。

## 一句话版本

| 系统 | 所属包 | 回答的问题 | 失败码 |
| --- | --- | --- | --- |
| 结构体 tag | `validation` | 解码后的请求是否满足字段声明的规则？ | `validation.<field>.<rule>` |
| 接口 | `endpoint` | 这个请求作为整体对业务是否合理？ | `bad_request.validation` |

两者都归类为 `invalid_argument`，因此都映射为 HTTP 400 与 gRPC
`InvalidArgument`。客户端不需要区分它们；读日志的人需要，而错误码点名了字段。

## Tag 校验

```go
type CreateUserRequest struct {
    Status string `json:"status" validate:"enum=active|suspended,default=active"`
    Age    int    `json:"age"    validate:"min=0,max=150"`
    Name   string `json:"name"   validate:"minLen=1,maxLen=64"`
}
```

`validation.Middleware()` 遍历请求的 `validate` tag，拒绝违反任一规则的值。规则
集刻意很小——`enum`、`min`、`max`、`minLen`、`maxLen`——因为每条规则与一个
JSON Schema 关键字一一对应：一个 tag 同时驱动生成的契约、前端据此构建的表单、
服务端运行的检查。`default` 由契约作为表单预填提示携带，校验不执行它。
`pattern` 规则刻意缺席：Go 结构体 tag 无法可靠携带反斜杠，而表单需要的正则
满是反斜杠。

nil 指针字段视为缺席——约束不在其上运行——因为"是否必填"是字段自己的事：
非指针字段在生成的契约里本就是必填。

用 `kit.WithEndpointMiddleware(validation.Middleware())` 接入；typed JSON 路由与
生成的传输层会自行校验。

## 接口校验

当规则需要多个字段——"end 必须晚于 start"、"两者恰居其一"——让请求实现
`endpoint.Validatable`：

```go
func (r CreateBookingRequest) Validate() error {
    if !r.End.After(r.Start) {
        return endpoint.NewValidationError("end", "must be after start")
    }
    return nil
}

ep := endpoint.NewBuilder(createBooking).
    WithValidation().
    Build()
```

`endpoint.ValidationMiddleware()` 在 endpoint 运行前调用 `Validate()`，收集
`endpoint.ValidationError` 的字段错误。传输层把失败编码为 400，错误码
`bad_request.validation`，错误体携带字段列表。

## 怎么选

- **单字段的形状规则**优先用 tag：枚举、区间、长度。生成的 OpenAPI 与 JSON
  Schema 会在客户端发送之前就把规则交给它。
- 契约表达不了的**跨字段与业务侧规则**用接口。
- 两者可组合：tag 管形状，接口管语义，都作为 endpoint 中间件运行，顺序不限。

精确的 tag 文法见 [`validation` 包](../validation/validation.go) godoc；错误码
如何到达每种传输见[错误处理](errors_zh.md)。
