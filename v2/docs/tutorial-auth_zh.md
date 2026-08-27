# 教程：认证中间件

[English](tutorial-auth.md) | 简体中文

本教程为一个 kit 服务添加 Bearer 密钥认证与基于角色的授权。认证在设计上由应用
自有；框架提供中间件边界与错误分类。完整代码即可运行的
[examples/auth](../examples/README_zh.md) 示例。

## 1. 目标

| 路由 | 行为 |
| --- | --- |
| `GET /health` | 公开 |
| `POST /api/me` | 任意有效密钥：200 并返回调用方身份 |
| `GET /api/admin` | 需要 admin 角色：否则 403 |
| 其他一切 | 密钥缺失或未知：401 |

## 2. 身份

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

## 3. 中间件

认证中间件校验 `Authorization` 头并注入身份。它通过 `kit.WithHTTPMiddleware`
以服务级方式安装，因此包裹除公开前缀之外的每一条路由：

```go
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

## 4. 单个路由上的授权

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

## 5. 装配

```go
svc, err := kit.NewHTTP(":8080",
	kit.WithHTTPMiddleware(authenticate(apiKeys)),
	kit.WithRequestID(),
)
host, err := kit.NewHost(kit.WithLifecycle(svc))
```

运行它，并尝试目标表格中的四行：

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
