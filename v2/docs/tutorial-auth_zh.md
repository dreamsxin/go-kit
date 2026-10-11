# 教程：认证中间件

[English](tutorial-auth.md) | 简体中文

本教程为一个 kit 服务添加 Bearer 密钥认证与基于角色的授权。认证在设计上由应用
自有；框架提供中间件边界与错误分类。下面每个代码块都是完整的——按节顺序操作，
`main.go` 就能拼装成可运行的服务，与可运行的
[examples/auth/main.go](../examples/auth/main.go) 示例是同一个文件，示例只多一段
更长的包注释。

## 1. 目标

| 路由 | 行为 |
| --- | --- |
| `/health`、`/livez`、`/readyz` | 公开 |
| `/api/me` | 任意有效密钥：200 并返回调用方身份 |
| `/api/admin` | 需要 admin 角色：否则 403 |
| 其他一切 | 密钥缺失或未知：401 |

这三条健康路由由 `kit.NewHTTP` 自己注册，因此公开前缀列表必须覆盖全部三条——
不只是 `/health`。

认证分为两个边界：HTTP middleware 从线路请求提取凭证；与传输无关的 `security`
包可以把凭证解析为 `security.Subject`，并在 endpoint middleware 中执行路由要求
（`security.Middleware`、`security.RequireAuthenticated`）——那种形态让 service
不依赖 bearer header、cookie 或某一种协议，见
[security/http README](../security/http/README_zh.md)。本教程改为把线路决策保留
在朴素的 HTTP middleware 里以便直接查看；它拼装的所有文件只依赖标准库与 `kit`。

## 2. 建立模块

```bash
mkdir authsvc && cd authsvc
go mod init example.com/authsvc
go get github.com/dreamsxin/go-kit/v2@latest
```

创建 `main.go`，写入包子句和后续每一节都要用到的 import：

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
)
```

## 3. 身份

身份是一个经由 context 传递的普通值。追加类型、它的 context helper、真实服务会
换成数据库或目录查询的密钥表，以及公开前缀列表：

```go
// identity 是已认证的调用方。authenticate 中间件从凭证解析出它；
// handler 与 requireRole 中间件消费它。
type identity struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

func (i identity) hasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type identityKey struct{}

func withIdentity(ctx context.Context, id identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func identityFromContext(ctx context.Context) (identity, bool) {
	id, ok := ctx.Value(identityKey{}).(identity)
	return id, ok
}

// apiKeys 把 bearer 凭证映射为身份。真实服务会从数据库、目录服务或
// 令牌内省端点解析身份。
var apiKeys = map[string]identity{
	"reader-key": {Subject: "reader", Roles: []string{"reader"}},
	"admin-key":  {Subject: "admin", Roles: []string{"reader", "admin"}},
}

// publicPrefixes 列出无需凭证即可访问的路由。
var publicPrefixes = []string{"/health", "/livez", "/readyz"}

func isPublic(path string) bool {
	for _, prefix := range publicPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
```

## 4. 中间件

认证中间件校验 `Authorization` header 并注入身份。它通过 `kit.WithHTTPMiddleware`
安装在整个服务上，包裹整个 mux——没有按路由排除的选项，所以哪些请求公开由
中间件自己决定：

```go
// authenticate 返回校验 Authorization header 并把身份注入请求 context 的
// HTTP 中间件。用 kit.WithHTTPMiddleware 安装在整个服务上。
func authenticate(keys map[string]identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			const prefix = "Bearer "
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, prefix) {
				writeAuthError(w, apperror.New(
					apperror.KindUnauthenticated,
					"auth.bearer_required",
					"missing bearer token",
				))
				return
			}
			id, ok := keys[strings.TrimPrefix(header, prefix)]
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

失败用 `apperror` 分类，因此响应携带稳定的机器可读代码而不是散文。

## 5. 单条路由上的授权

角色检查包裹单条路由，在认证之后运行。追加中间件，以及它与 `authenticate` 共享
的错误渲染器：

```go
// requireRole 返回只放行持有指定角色身份的 HTTP 中间件。
// 用它包裹单条路由的 handler。
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

// writeAuthError 把已分类的 apperror 渲染成 HTTP 传输使用的 JSON 错误形态，
// HTTP 状态码由 kind 推导。
func writeAuthError(w http.ResponseWriter, err *apperror.Error) {
	status := http.StatusInternalServerError
	switch err.ErrorKind() {
	case apperror.KindUnauthenticated:
		status = http.StatusUnauthorized
	case apperror.KindPermissionDenied:
		status = http.StatusForbidden
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    err.ErrorCode(),
		"message": err.PublicMessage(),
	})
}
```

## 6. 装配

追加装配代码，文件就完整了。`Handle` 接受裸路径而不是 `"GET /api/admin"` 模式，
因此 admin 路由响应所有方法；想让 mux 替你拒绝其他方法，就在模式里加上动词：

```go
func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	svc, err := kit.NewHTTP(*httpAddr,
		kit.WithHTTPMiddleware(authenticate(apiKeys)),
		kit.WithRequestID(),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 读取注入身份的类型化 JSON 端点。
	type meRequest struct{}
	kit.HandleJSONTyped(svc, "/api/me", func(ctx context.Context, _ meRequest) (identity, error) {
		id, ok := identityFromContext(ctx)
		if !ok {
			return identity{}, apperror.New(
				apperror.KindUnauthenticated,
				"auth.no_identity",
				"request was not authenticated",
			)
		}
		return id, nil
	})

	// 原生 handler 上的路由级授权。
	svc.Handle("/api/admin", requireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"welcome"}`))
	})))

	log.Println("auth example listening on", *httpAddr)

	host, err := kit.NewHost(kit.WithLifecycle(svc))
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

## 7. 运行它

在教程模块里运行你刚拼装好的文件，并逐一尝试目标表中的每一行：

```bash
go run .
curl -i -X POST http://localhost:8080/api/me -d '{}'                                  # 401
curl -H "Authorization: Bearer reader-key" -H "Content-Type: application/json" \
     -X POST http://localhost:8080/api/me -d '{}'                           # 200
curl -i -H "Authorization: Bearer reader-key" http://localhost:8080/api/admin          # 403
curl -H "Authorization: Bearer admin-key" http://localhost:8080/api/admin              # 200
curl http://localhost:8080/health                                                      # 200
```

同一个服务也可以在仓库检出里从 `v2/` 模块目录用 `go run ./examples/auth` 运行。

## 接下来去哪

- [中间件](middleware_zh.md)：组合与流控
- [错误处理](errors_zh.md)：分类与线上格式
- [security/http](../security/http/README_zh.md)：面向浏览器服务的 CORS、CSRF 与
  IP 策略
