# 可选的 HTTP 安全中间件

[English](README.md) | 简体中文

`security/http` 提供可组合的标准库 `http.Handler` 中间件。它不定义认证、授权、身份或部署策略。

```go
proxy, err := httpsecurity.NewTrustedProxy(httpsecurity.TrustedProxyConfig{
    TrustedProxies: []string{"10.0.0.0/8"},
})
ipPolicy, err := httpsecurity.NewIPPolicy(httpsecurity.IPPolicyConfig{
    Allow: []string{"203.0.113.0/24"},
})
cors, err := httpsecurity.NewCORS(httpsecurity.CORSConfig{
    AllowedOrigins:   []string{"https://app.example.com"},
    AllowedMethods:   []string{"GET", "POST"},
    AllowCredentials: true,
})
headers, err := httpsecurity.NewSecurityHeaders(httpsecurity.SecurityHeadersConfig{
    StrictTransportSecurity: "max-age=31536000; includeSubDomains",
})
csrf, err := httpsecurity.NewCSRF(httpsecurity.CSRFConfig{
    Secret:        csrfSecretFromEnvironment,
    RequireOrigin: true,
    SecureCookie:  true,
})

handler := httpsecurity.Chain(
    proxy,
    headers,
    ipPolicy,
    cors,
    csrf,
)(applicationHandler)
```

在 `kit` 中，为健康检查、JSON 端点、原生 HTTP 以及生成的路由一次性安装同一组策略即可：

```go
service, err := kit.New(":8080",
    kit.WithHTTPMiddleware(proxy, headers, ipPolicy, cors, csrf),
)
```

每个构造函数在返回中间件之前都会校验配置。除非应用有特定的理由需要更改，否则请保持推荐顺序：

1. 可信代理解析确定实际的客户端 IP 和协议 scheme。
2. 安全响应头覆盖后续的策略响应，并使用可信的 scheme 处理仅限 HTTPS 的 HSTS。
3. IP 策略基于实际的客户端 IP 进行评估。
4. CORS 在 CSRF 校验之前应答浏览器预检请求。
5. CSRF 只保护位于其内部、使用 cookie 认证的应用路由。

## 信任边界

- 除非直接对端位于 `TrustedProxies` 中，否则转发头会被忽略。只配置那些会覆写或正确追加 `X-Forwarded-For` 和 `X-Forwarded-Proto` 的代理。
- 拒绝网络的优先级高于允许网络。非空的允许列表会拒绝所有未匹配的地址。
- CORS origin 必须是精确的 HTTP(S) origin。通配符 origin 不能与凭证（credentials）同时使用。
- CSRF 使用经 HMAC 签名的双重提交（double-submit）cookie。其至少 32 字节的密钥应从应用的密钥管理中加载，并且只作用于真正使用浏览器 cookie 做认证的路由。基于 Bearer 令牌的 API 通常不需要这个中间件。
- 认证与业务授权仍然属于应用装配以及 endpoint/service 逻辑的职责范围。

## 流式传输与 MCP

这些中间件在调用被包装的 handler 之前写入响应头，并且从不替换 `http.ResponseWriter`，因此 `Flusher`、`Hijacker` 和流式行为仍然可用。CORS 因而可以包装 SSE 路由。

CSRF 不应全局安装在 MCP 或其他非浏览器 POST 协议之上。只有当这些路由确实使用浏览器 cookie，并且客户端能够获取安全请求 cookie 并回显令牌头时才应用它。把 CORS 放在 CSRF 外层，这样浏览器预检请求就无需携带令牌。
