# security

与传输无关的已认证主体契约。

[English](README.md) | 简体中文

本包回答一个问题：一旦协议边界确立了**调用方是谁**，这个主体如何跨层传递到
endpoint 与服务层，以及由谁执行粗粒度要求——"已认证"、"持有角色"。

它有意**不**提取或校验凭证。那属于协议边界（应用自己的 HTTP middleware，或者用
[`security/http`](../security/http/README_zh.md) 处理 CORS、CSRF、header 与 IP
策略）。主体与角色之外的业务授权规则属于服务层。

## 职责划分

| 层 | 负责 |
| --- | --- |
| `security/http` 或你的 HTTP middleware | 凭证提取与校验——协议相关 |
| **本包** | 主体传递与 endpoint 级执行——与传输无关 |
| 服务层 | 主体与角色之外的业务授权规则 |

## Subject

`Subject` 是已认证的调用主体：

```go
type Subject struct {
	ID     string
	Kind   SubjectKind // SubjectAnonymous | SubjectUser | SubjectService
	Roles  []string
	Claims map[string]any // 认证器提供；认证之后视为只读
}
```

`HasRole(role)` 报告角色成员资格；`Authenticated()` 报告该主体是否指认了一个主
体——有 ID 且非匿名。出现在 context 里**不等于**已认证：`Middleware` 也会存匿名
主体，所以判断永远用 `Authenticated()`，不要只看 context 查找是否成功。

## 确立主体

`Authenticator` 从传输边界已放入 context 的凭证解析出主体：

```go
authenticator := security.AuthenticatorFunc(func(ctx context.Context) (security.Subject, error) {
	token, _ := ctx.Value(bearerKey{}).(string) // 由你的 HTTP middleware 放入
	id, ok := keys[token]
	if !ok {
		return security.Subject{}, apperror.Unauthenticated("auth.invalid", "credentials are missing or invalid")
	}
	return security.Subject{ID: id.Subject, Kind: security.SubjectUser, Roles: id.Roles}, nil
})
```

失败用 `apperror` 分类：`KindUnauthenticated` 映射到 HTTP 401（gRPC
`Unauthenticated`），`KindPermissionDenied` 映射到 403。返回零值主体加 nil 错误
表示该请求匿名——中间件会存下这个结果，下面的执行中间件据此拒绝。

边界代码用 `security.WithSubject(ctx, subject)` 存储结果；读取方用
`security.SubjectFromContext` 并检查 `Authenticated`。

## 在 endpoint 执行

`security.Middleware(auth)` 是端点中间件，在被包裹的 endpoint 之前运行认证器并
注入其结果。要求类中间件随后检查它：

- `security.RequireAuthenticated()`——没有已认证主体则拒绝
  （`security.unauthenticated`，HTTP 401）；
- `security.RequireRole("admin")`——在此基础上还要求角色
  （`security.role_required`，HTTP 403）。

在装配 endpoint 的地方组合它们：

```go
svc, err := kit.NewHTTP(addr,
	kit.WithEndpointMiddleware(
		security.Middleware(authenticator),
		security.RequireAuthenticated(),
	),
)

// 公开路由不加链；admin 路由按路由追加角色检查：
kit.HandleJSONTypedWithMiddleware(svc, "POST /api/admin", adminHandler,
	func(b *endpoint.Builder) *endpoint.Builder {
		return b.Use(security.RequireRole("admin"))
	})
```

组件注册的健康路由等公开端点在链之外，因此也在执行之外。

对 MCP，同样的契约作用于传输层：`Authorizer` 读取
`security.SubjectFromContext` 并按方法决策——见
[interaction README](../interaction/README_zh.md) 与 MCP 教程。

## 接下来去哪

- [认证教程](../docs/tutorial-auth_zh.md)：与本包组合的应用自有 HTTP 边界
- [`security/http`](../security/http/README_zh.md)：CORS、CSRF、安全 header、
  可信代理、IP 策略
- [错误处理](../docs/errors_zh.md)：分类如何映射到状态码
