# 教程：认证中间件

[English](tutorial-auth.md) | 简体中文

本教程为一个 kit 服务添加 Bearer 密钥认证与基于角色的授权。认证在设计上由应用
自有；框架提供中间件边界与错误分类。完整代码即可运行的
[examples/auth/main.go](../examples/auth/main.go) 示例。

## 1. 目标

| 路由 | 行为 |
| --- | --- |
| `/health`、`/livez`、`/readyz` | 公开 |
| `/api/me` | 任意有效密钥：200 并返回调用方身份 |
| `/api/admin` | 需要 admin 角色：否则 403 |
| 其他一切 | 密钥缺失或未知：401 |

这三条健康路由由 `kit.NewHTTP` 自己注册，因此公开前缀列表必须覆盖全部三条——
不只是 `/health`。

## 2. 标准分层

认证分为两个边界：HTTP middleware 从线路请求提取凭证；与传输无关的 `security`
包把凭证解析为 `security.Subject`，并在 endpoint middleware 中执行路由要求。
这样 service 不依赖 bearer header、cookie 或某一种协议。

```go
type credentialKey struct{}

func extractBearer(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
        ctx := context.WithValue(r.Context(), credentialKey{}, token)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

type bearerAuthenticator struct{ keys map[string]identity }

func (a bearerAuthenticator) Authenticate(ctx context.Context) (security.Subject, error) {
    token, _ := ctx.Value(credentialKey{}).(string)
    id, ok := a.keys[token]
    if !ok || token == "" {
        return security.Subject{}, apperror.Unauthenticated(
            "auth.invalid", "credentials are missing or invalid",
        )
    }
    return security.Subject{
        ID: id.Subject, Kind: security.SubjectUser, Roles: id.Roles,
    }, nil
}
```

把提取器安装在 HTTP 边界，把解析器安装在 endpoint 边界。公开健康路由是组件的
raw 路由，因此不受保护；私有 JSON 路由显式添加认证要求：

```go
svc, _ := kit.NewHTTP(":8080",
    kit.WithHTTPMiddleware(extractBearer),
    kit.WithEndpointMiddleware(security.Middleware(bearerAuthenticator{keys: apiKeys})),
)

kit.HandleJSONTypedWithMiddleware(svc, "POST /api/me", meHandler,
    func(b *endpoint.Builder) *endpoint.Builder {
        return b.Use(security.RequireAuthenticated())
    })
```

需要角色控制时使用 `security.RequireRole("admin")`。依赖资源所有权或业务状态的
授权仍应放在 service 层。下面继续用小型应用 HTTP helper 展示同一流程，以便看清
线路层的决策。

## 3. 身份

身份是一个通过 context 传递的普通值：

```go
type identity struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

func identityFromContext(ctx context.Context) (identity, bool) {
	id, ok := ctx.Value(identityKey{}).(identity)
	return id, ok
}
```

## 4. 中间件

认证中间件校验 `Authorization` 头并注入身份。它通过 `kit.WithHTTPMiddleware`
以服务级方式安装，包裹整个 mux——没有按路由排除的选项，因此由中间件自己决定
什么是公开的：

```go
var publicPrefixes = []string{"/health", "/livez", "/readyz"}

func authenticate(keys map[string]identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				writeAuthError(w, apperror.New(
					apperror.KindUnauthenticated,
					"auth.bearer_required",
					"missing bearer token",
				))
				return
			}
			id, ok := keys[strings.TrimPrefix(header, "Bearer ")]
			if !ok {
				writeAuthError(w, apperror.New(
					apperror.KindUnauthenticated,
					"auth.unknown_key",
					"unknown credentials",
				))
				return
			}
			next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
		})
	}
}
```

失败用 `apperror` 分类，因此响应携带稳定的机器可读码，而不是一段散文。

## 5. 单个路由上的授权

角色检查在认证运行之后包裹单个路由：

```go
func requireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := identityFromContext(r.Context())
			if !ok || !id.hasRole(role) {
				writeAuthError(w, apperror.New(
					apperror.KindPermissionDenied,
					"auth.role_required",
					"caller does not hold role "+role,
				))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

svc.Handle("/api/admin", requireRole("admin")(adminHandler))
```

`Handle` 接受的是裸路径，而不是 `"GET /api/admin"` 这样的模式，因此该路由对所有
方法都响应。如果希望 mux 帮你拒绝其他方法，请把动词写进模式里。

## 6. 装配

```go
httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
flag.Parse()

ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

svc, err := kit.NewHTTP(*httpAddr,
	kit.WithHTTPMiddleware(authenticate(apiKeys)),
	kit.WithRequestID(),
)
if err != nil {
	log.Fatal(err)
}
registerRoutes(svc)

host, err := kit.NewHost(kit.WithLifecycle(svc))
if err != nil {
	log.Fatal(err)
}
if err := host.Run(ctx); err != nil {
	log.Fatal(err)
}
```

从 `v2/` 模块目录运行它，并尝试目标表格中的各行：

```bash
go run ./examples/auth
curl -i -X POST http://localhost:8080/api/me -d '{}'                                  # 401
curl -H "Authorization: Bearer reader-key" -X POST http://localhost:8080/api/me -d '{}'  # 200
curl -i -H "Authorization: Bearer reader-key" http://localhost:8080/api/admin          # 403
curl -H "Authorization: Bearer admin-key" http://localhost:8080/api/admin              # 200
curl http://localhost:8080/health                                                      # 200
```

## 接下来去哪

- [中间件](middleware_zh.md)：组合与流控
- [错误处理](errors_zh.md)：分类与线上格式
- [security/http](../security/http/README_zh.md)：面向浏览器的服务的 CORS、CSRF
  与 IP 策略
