# microgen 用户指南

[English](MICROGEN.md) | 简体中文

`microgen` 从契约生成可直接运行的 Go 服务。它是新服务的推荐入口；运行时
各包仍可独立使用。

本指南对应主 module 中发布的 `microgen` package，安装方式：

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@latest
```

## 目录

- [快速选择](#快速选择)
- [安装](#安装)
- [源模式](#源模式)
- [常用选项](#常用选项)
- [生成项目清单](#生成项目清单)
- [API 与 Schema 契约](#api-与-schema-契约)
- [声明式认证](#声明式认证)
- [依据契约生成前端](#依据契约生成前端)
- [最小生成与完整生成](#最小生成与完整生成)
- [生成配置](#生成配置)
- [在生成项目中使用框架组件](#在生成项目中使用框架组件)
- [数据库迁移](#数据库迁移)
- [扩展既有项目](#扩展既有项目)
- [文件所有权](#文件所有权)
- [生成之后](#生成之后)
- [生成器保证](#生成器保证)

## 快速选择

| 来源 | 命令形式 | 结果 |
| --- | --- | --- |
| Go 接口和结构体 | `microgen -idl idl.go ...` | HTTP、可选 gRPC、客户端、SDK、契约文件 |
| Protobuf | `microgen -idl service.proto ...` | 生成 proto 传输和客户端 |
| 已有数据库 | `microgen -from-db -driver ... -dsn ...` | 模型、仓库和服务骨架 |

OpenAPI 输出默认开启——每个生成项目都会应答机器可读契约、Swagger UI 与
TypeScript 客户端，这正是 AI 代理生成前端所需的全部输入（见[依据契约生成前端]
(#依据契约生成前端)）。除此之外仍从最小能力集开始：只有项目确实需要时再加
`-config`、`-db`、`-model` 或 `-interaction`（`-openapi=false` 关闭契约输出）。
生成后先读项目 README 和 `.microgen/manifest.json`，再开始编辑。

## 安装

在 v2 开发过程中，从仓库根目录执行：

```bash
go -C v2 install ./cmd/microgen
```

CLI 无需 CGO，包括 SQLite schema 内省。安装与运行 `microgen` 不需要 GCC。
生成的服务仍可在自己的 `-db` 设置中显式选择依赖 CGO 的运行时数据库
适配器。

查看当前检出版本支持的准确 CLI：

```bash
microgen -h
microgen extend -h
```

## 源模式

初始生成必须且只能选择一种源模式。

### Go IDL

```bash
microgen -idl idl.go -out ./service -import example.com/service
```

输入包含服务接口与请求/响应类型。`microgen` 把契约复制进生成项目，并把
方法映射到统一 IR；HTTP、客户端、SDK、OpenAPI 3.1、JSON Schema 2020-12
以及可选 MCP 工具都由这份 IR 驱动。

### Protobuf

```bash
microgen \
  -idl service.proto \
  -out ./service \
  -import example.com/service \
  -protocols http,grpc
```

先审查 `pb/` 下的生成文件，再用生成 README 中写明的 `protoc` 命令生成
Go stub。支持的一元与流式 gRPC 形态同样来自解析后的契约。

### 数据库模式

```bash
microgen \
  -from-db \
  -driver mysql \
  -dsn 'user:pass@tcp(localhost:3306)/catalog' \
  -dbname catalog \
  -tables users,products \
  -out ./catalog-svc \
  -import example.com/catalog-svc
```

数据库内省支持 MySQL、PostgreSQL 与 SQLite。它对源数据库只读。生成的
模型保留发现的列，且 `-from-db` 时总是输出；数据库运行时接线仍需通过
`-db` 显式开启，启动迁移默认关闭。

## 常用选项

| 选项 | 含义 |
| --- | --- |
| `-idl` | IDL 文件路径（`.go` 或 `.proto`） |
| `-from-db` | 从数据库模式生成，而不是 IDL 文件 |
| `-dsn` | 数据库 DSN，`-from-db` 必填 |
| `-dbname` | 数据库名，`-from-db` 必填 |
| `-tables` | 逗号分隔的待内省表名，仅 `-from-db` |
| `-out` | 输出目录 |
| `-import` | 生成的 Go module 路径 |
| `-service` | 生成的服务名；默认取契约中的第一个服务 |
| `-protocols` | `http` 或 `http,grpc` |
| `-prefix` | HTTP 路由前缀 |
| `-config` | 生成配置支持，默认 `false` |
| `-config-mode` | `file`、`hybrid` 或 `remote` |
| `-remote-provider` | 远程配置提供方；当前支持 `consul` |
| `-db` | 生成数据库运行时接线，默认 `false` |
| `-driver` | `-from-db` 与 `-db` 的数据库驱动，默认 `mysql` |
| `-model` | 生成 model/repository 输出，默认 `false` |
| `-docs` | 生成项目文档，默认 `true` |
| `-tests` | 生成项目测试 |
| `-interaction` | 生成交互运行时与 `/mcp` 端点 |
| `-openapi` | 生成 OpenAPI 3.1、JSON Schema、Swagger UI 与 TypeScript SDK；默认 `true` |

以 `microgen -h` 为权威选项列表。

## 生成项目清单

每次完整生成都会写入 `.microgen/manifest.json`，schema 版本为
`microgen.project.v3`。它是生成项目的首要身份与再生成契约，记录：

- 源模式与 Go module 路径；
- 已启用的能力，以及配置模式、远程提供方与数据库驱动；
- 路由前缀、服务、生成的模型与生成中间件顺序；
- 所有生成器持有产物的规范化相对路径。

不要手工编辑清单。完整生成会在写完其余产物后刷新它。服务、模型与中间件
extend 操作最后刷新清单，因此不完整的写入不会声称完成了一次生成。

`microgen extend -check -out .` 校验清单 schema 与 module 路径，把声明的
产物与文件系统和所有权规则比对，报告缺失、未声明或类型不匹配的输出。
清单缺失或存在漂移时，extend 变更会被拒绝。

`microgen.project.v3` 之前生成的项目仍使用返回生成器内部路由条目的旧
`registerCustomRoutes` 钩子。extend 会拒绝它们并给出迁移提示：把
`cmd/custom_routes.go` 改为 `func registerCustomRoutes(r *http.ServeMux)`
（直接在 mux 上注册路由；把清单条目移入 `customRouteDescriptions() []string`），
同步更新自定义的 `cmd/main.go`，然后重新完整生成以刷新清单。

## API 与 Schema 契约

用 `-openapi` 启用契约输出。生成的 API 契约为 OpenAPI 3.1，可复用消息
schema bundle 使用 JSON Schema 2020-12。Swagger UI 5 为内嵌查看器，不需要
CDN 访问。同时生成不依赖运行时的、基于 Fetch 的 TypeScript 源码 SDK。

`microgen` 写入：

- `docs/openapi.json`：路径、操作、请求/响应 schema，以及直接从统一 IR
  生成的 `components.schemas`；
- `docs/schema.json`：`$defs` 下的可复用消息 schema，与 OpenAPI 使用同一
  schema 构建器生成；
- `docs/docs.go`：同时提供两份生成契约的内嵌包装；
- `sdk/typescript/client.ts`：类型化一元 HTTP 客户端、消息接口、请求
  取消、头部与非 2xx 错误；
- `sdk/typescript/tsconfig.json`：严格的 TypeScript 编译器设置，用生成
  SDK README 中记录的发布固定版本检查；
- `sdk/typescript/README.md`：生成的用法与类型检查说明；
- `GET /openapi.json`：运行时契约端点；
- `GET /schema.json`：运行时 JSON Schema 端点；
- `GET /swagger/`：读取 `/openapi.json` 的 Swagger UI。

HTTP 传输文件不包含重复的注解契约。生成文档使用相对 URL，因此生成配置
没有 `swagger_host` 设置。生成与 extend 模式会刷新 `docs/` 与
`sdk/typescript/` 下的两个目录；把它们视为生成器持有。生成文本为无 BOM
的 UTF-8。流式 RPC 保留在生成的 Go gRPC SDK 上。

### schema 关于线上格式说了什么

有三条规则把生成的文档和生成的 handler 连起来，于是照文档做出来的客户端，
能对上同一次生成产出的那个服务：

- `additionalProperties` 是 `false`。请求体用 `DisallowUnknownFields` 解码，
  未知属性就是 400；文档如果闭口不谈，含义就是"允许多余属性"，于是生成出来的
  客户端会被它自己的服务拒绝。在响应 schema 上，这是同一句话的另一面：就是
  这些属性，没有别的。
- 指针字段可为 null。生成的结构体不带 `omitempty`，所以 nil 指针会 marshal 成
  `null`。普通类型变成两元素的类型列表（`["string", "null"]`）；`$ref` 则包进
  `anyOf`，因为并非所有实现都认 `$ref` 旁边的关键字。
- `required` 由类型推导：非指针即 required，而"声明为指针"就是声明可选。它是
  关于载荷的陈述，不是"服务端会拒绝缺省"的承诺——非指针字段上无法把"缺省"和
  "零值"区分开，于是缺失的 `count` 到达时就是 `0`。当"缺省"必须区别于"零"时，
  把这个字段声明成指针。

仓库契约验证会解析每份生成的 OpenAPI 3.1 文档、编译每条 JSON Schema
2020-12 定义、用固定版本编译器对生成客户端做类型检查、比对 Go 与
TypeScript SDK 的 HTTP 行为，并检查 Go IDL、Protobuf 与数据库源的已评审
契约快照。从 `v2` 执行：

```bash
make test-contracts
```

TypeScript 步骤需要 Node.js 与 `npx`；生成的服务不会引入 Node.js 运行时
依赖。

当有意的公开契约变更更新了生成产物时，先评审 diff，再只刷新受影响的
快照：

```bash
go test ./tools -run "TestMicrogen(IDLContractIntegration|ProtoIntegration|FromDBIntegration)" \
  -count=1 -args -update-contract-snapshots
```

生成的 Go SDK 对非 2xx 响应返回导出的 `APIError`，其 `StatusCode` 与
`Body` 字段和 TypeScript SDK 的错误契约一致。它通过 `net/url` 解析请求
路径，默认把响应体限制在 4 MiB；契约需要其他上限时使用
`WithMaxResponseBodyBytes`。`client/<service>/` 下的可运行文件委托给该
SDK，而不是维护第二套 HTTP 或 gRPC 实现。

生成的 repository 通过模型派生的字段白名单解析 `order_by`。不支持的名称
返回错误，而不是透传给 SQL。

### 字段约束

字段可以携带 `validate` tag——一个小心选定的规则集，每条规则一一映射到 JSON
Schema 关键词：一个 tag、一条规则、三个读者：契约记载它，生成的表单用它校验，
生成的服务（以及类型化的 kit 路由）执行它：

| Tag | 契约关键词 | 表单效果 |
| --- | --- | --- |
| `enum=active\|suspended` | `enum` | 列出选项的下拉框 |
| `min=0,max=150` | `minimum` / `maximum` | 数值范围 |
| `minLen=1,maxLen=64` | `minLength` / `maxLength` | 字符串长度 |
| `default=active` | `default` | 表单预填值 |

```go
type CreateUserRequest struct {
	Username string `json:"username" validate:"minLen=1,maxLen=64"`
	Status   string `json:"status"   validate:"enum=active|suspended,default=active"`
}
```

生成的传输对每个解码后的请求执行校验，违规以 400 应答，code 指名字段与规则
（`validation.username.minLen`）；类型化的 kit 路由经同一个包执行相同 tag，手写
服务与生成服务以同样的错误拒绝同样的请求。`default` 是契约携带的表单提示——任
何东西都不执行它。从数据库生成的项目直接从 DDL 推导约束：`varchar(n)` 列界定字
符串长度，`enum(...)` 列列出取值，无需写任何 tag。tag 里的未知规则或畸形值在生成
期即被拒绝。

### 声明式认证

方法的文档注释可以声明它的认证要求，一句声明到达三处：契约记载它（每操作的
`security` 与 401/403 应答、`components.securitySchemes` 中的 http bearer
scheme）、生成的传输执行它、依据文档生成的表单知道该路由需要登录调用方：

```go
// DeleteUser removes a user.
//
// auth: roles=admin
DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)

// UpdateUser modifies a user.
//
// auth: required
UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
```

`auth: required` 要求已认证主体；`auth: roles=a,b` 要求主体持有任一所列角色
（any-of）。两个指令可写在同一行。未声明的方法没有生成的强制——保护整个表面仍
是部署自己的决策，在 `cmd/main.go` 或 custom chain 中做出。

强制需要一个认证器，而认证器是你的：生成的 `service/<name>/service.go` 声明了
`NewAuthenticator() security.Authenticator`，未实现时返回 nil。fail-closed 是契
约的承诺：钩子返回认证器之前，每个受保护方法都以 401 `auth.not_configured`
应答——绝不会有请求因疏忽而未认证地运行。凭证交换本身（bearer token、API
key）留在应用的 HTTP 边界；见[认证教程](docs/tutorial-auth_zh.md)与 `security`
包。

认证注解之前生成的项目还没有该钩子——把这两个函数加进 `service.go`（或重新生
成到新目录）就是全部迁移；在那之前，带注解的方法持续 fail-closed。

## 依据契约生成前端

契约的书写目标就是让 AI 代理——或开发者——无需阅读 Go 源码即可构建前端，包
括后台管理：

- **操作**：`docs/openapi.json` 列出每条路由及其 `operationId`
  （`<Service>_<Method>`）、来自 IDL 注释的 summary，以及作为 `components.schemas`
  `$ref` 的请求/响应体。
- **消息形态**：`docs/schema.json` 在 `$defs` 下承载相同 schema。每个字段标注
  `type` 与 `format`（`int64`、`date-time`、`uuid`）、`example` 与对象的 `required`
  清单，且每个消息都设置 `additionalProperties: false`——按声明的字段渲染，一个
  不多。
- **调用**：`sdk/typescript/client.ts` 暴露 `APIClient`，每个操作一个类型化方法，
  非 2xx 应答统一抛 `APIError`；按文档路由直接 `fetch` 亦可。

可复用的页面配方：从 `List`/`Get` 操作得到列表页，从 `Create`/`Update` 请求
schema 得到表单（`required` 驱动校验、`format` 驱动输入控件），从 `Delete` 操作
得到删除确认。文档中的每条路径已含服务前缀。生成的 `.ai/PROJECT_GUIDE.md` 为在
项目内工作的代理携带同一工作流。

## 最小生成与完整生成

最小 HTTP 项目：

```bash
microgen \
  -idl idl.go \
  -out . \
  -import example.com/service \
  -protocols http
```

带生成配置与测试的 HTTP + gRPC 项目：

```bash
microgen \
  -idl service.proto \
  -out . \
  -import example.com/service \
  -protocols http,grpc \
  -config=true \
  -config-mode=file \
  -tests
```

交互/MCP 项目：

```bash
microgen \
  -idl idl.go \
  -out . \
  -import example.com/agent-service \
  -interaction
```

MCP 使用长时 HTTP 响应。生成项目默认
`read_header_timeout: 5s`、`write_timeout: 0s`；只有在大于预期的最大流
时长时才设置有限的写超时。

## 生成配置

`-config=true` 时，运行时配置按以下顺序加载：

```text
defaults
  -> local YAML
  -> environment bootstrap for remote connection settings
  -> optional remote config
  -> final environment overrides
  -> Config.Validate
```

环境变量使用 `APP_` 前缀，例如：

```text
APP_HTTP_ADDR
APP_LOG_LEVEL
APP_LOG_FORMAT
APP_DB_DSN
APP_DB_AUTO_MIGRATE
APP_REMOTE_ENABLED
```

应用特有的配置节放在用户持有的 `config/custom.go` 文件中。向
`CustomConfig` 添加字段；YAML 与远程配置合并进 `custom`，
`SetDefaults`、`ApplyEnv` 与 `Validate` 为自定义默认值、环境变量与校验
提供显式钩子。完整再生成永远不会覆盖该文件。

模式：

| 模式 | 行为 |
| --- | --- |
| `file` | 本地文件加环境变量；禁用远程加载 |
| `hybrid` | 启用远程加载，本地回退 |
| `remote` | 强制远程加载；远程出错时启动失败 |

示例：

```bash
microgen -idl idl.go -out . -import example.com/svc -config -config-mode=file

microgen \
  -idl idl.go \
  -out . \
  -import example.com/svc \
  -config \
  -config-mode=hybrid \
  -remote-provider=consul
```

最终合并的配置在创建 logger、数据库、中间件与服务器之前完成校验。不要
把凭据写进生成 YAML；通过部署环境或应用自有的提供方注入。

生成的 endpoint 链包含标准库日志、超时与所选的供应商中立生成中间件。
限流、熔断及其他持有依赖的适配器有意不加入新项目的 module 图。需要时
显式安装，并在用户持有的 `endpoint/<service>/custom_chain.go` 钩子中
组装。

## 在生成项目中使用框架组件

生成项目本身就是框架的装配体；加入你自己的使用，是一个**位置**问题——每个意图
都有唯一正确的位置：

| 你想…… | 位置 | 组件 |
| --- | --- | --- |
| 依赖故障时让就绪检查失败 | `cmd/main.go` | `kit.WithReadinessCheck` |
| 对端点限流、熔断或舱壁隔离 | `endpoint/<name>/custom_chain.go` | endpoint 中间件目录 |
| 执行认证与角色 | `endpoint/<name>/custom_chain.go` | `security` |
| 按路由观测结果 | `cmd/main.go` | `kit.WithMetrics`、`kit.WithRecorder` |
| 带重试或服务发现调用下游 | `service/<name>/service.go` | `sd` |
| 以类型化函数调用 HTTP API | `service/<name>/service.go` | `client.NewTypedJSONClient` |
| 装配所需的设置 | `config/custom.go` | 生成的配置 |

`cmd/main.go`、`service/<name>/service.go` 与 `custom_chain.go` 都是用户持有：
生成器只写一次、绝不覆盖，装配由你保留。

### custom_chain.go 中的端点中间件

该钩子包裹服务的每个端点，位于组件链内侧：

```go
func applyCustomMiddleware(ep endpoint.Endpoint, logger *slog.Logger, cfg MiddlewareConfig, name string) endpoint.Endpoint {
	ep = endpoint.NewCircuitBreaker().Middleware()(ep)
	ep = endpoint.RateLimitMiddleware(limiter)(ep)
	return ep
}
```

限流器本身归应用所有——任何具备 `Allow()` 与 `Wait(ctx)` 的类型都满足
`endpoint.RateLimiter`，`golang.org/x/time/rate` 的 `Limiter` 开箱即满足：

```go
var limiter = rate.NewLimiter(rate.Every(100*time.Millisecond), 20)
```

目录、顺序规则，以及每种中间件产生的错误到状态码映射，见[中间件章节]
(docs/middleware_zh.md)。

### 认证与角色

`security` 包承载与传输无关的执行：

```go
func applyCustomMiddleware(ep endpoint.Endpoint, logger *slog.Logger, cfg MiddlewareConfig, name string) endpoint.Endpoint {
	ep = security.Middleware(authenticator)(ep) // 解析主体
	ep = security.RequireAuthenticated()(ep)
	if name == "DeleteUser" {
		ep = security.RequireRole("admin")(ep)
	}
	return ep
}
```

`name` 是路由的操作名，单个钩子即可对破坏性操作做差异化限制。失败以分类错误到
达，与其他错误一样映射为 401 与 403。

### main.go 中的就绪检查与指标

组件在 `cmd/main.go` 中装配；检查与可观测性也加在那里：

```go
svc, err := kit.NewHTTP(*httpAddr,
	kit.WithRequestID(),
	kit.WithMetrics(&metrics),
	kit.WithReadinessCheck("database", func(ctx context.Context) error {
		return store.Ping(ctx)
	}),
)
```

就绪检查失败会把 `/readyz` 翻成 503——负载均衡器据此摘除实例。日志、追踪与基
数规则见[可观测性](docs/observability_zh.md)；带重试与健康检查实例的下游调用见
[服务发现](docs/service-discovery_zh.md)。

## 数据库迁移

生成的服务默认跳过 `AutoMigrate`。仅在有意进行启动期 schema 变更时开启：

```text
database.auto_migrate: true
APP_DB_AUTO_MIGRATE=true
```

生成的服务二进制还接受启动参数 `-auto-migrate`（见其 `-h` 输出）；它是
生成 `cmd` 二进制的参数，不是 `microgen` 本身的参数。

生产 schema 变更通常应使用专门的迁移流程。

## 扩展既有项目

项目自带完整契约：`idl.go`，与代码并列存放，并由每次 extend 更新。就地编辑它再
执行 extend，就是增量循环。

**刷新**——服务多了方法、类型变了、加了 validate 标签。编辑 `idl.go`，然后不带
任何追加标志运行 extend：能力来自 manifest，所有生成器持有的工件被刷新，你的文
件不会被写入：

```bash
microgen extend -out .
```

给服务加方法会触及你自己的 `service.go` 的三处——接口、实现、日志包装——编译器
会带你走完（报错先指向生成文件；修复永远在用户持有的文件里）。契约移除了什么，
会产生一条警告，指名不再生成的生成器持有工件；它们原地保留，因为删除文件是一个
决策，而不是副作用。下一次不带 `-idl` 的 extend 会读取 `idl.go`，而每次 extend
都会把它更新为本次运行所用契约。

**追加**——全新的服务、模型或中间件集：

```bash
microgen extend -append-service OrderService
microgen extend -append-model Product
microgen extend -append-middleware tracing,error-handling,metrics
```

三者同样接受 `-idl`——传入另一份完整合并的 Go IDL 契约，或让项目自带的
`idl.go` 应答。追加在触碰任何文件之前会校验来源覆盖了每个既有服务与模型，缺失
会被指名。追加同样会在契约输出启用时刷新 OpenAPI、JSON Schema 与 TypeScript
SDK；每次 extend 都会刷新 `.microgen/manifest.json`，对存在清单漂移或缺少必需
所有权接缝的项目拒绝执行。

需要只看报告不做改动时，先运行只读兼容性扫描：

```bash
microgen extend -check
```

extend 模式不支持 Protobuf 输入：契约副本是 Go 文件。

## 文件所有权

生成项目里的任何文件都可以用两个机器可查的标记判断归属。文件名以 `generated_`
开头，或首行为 `// Code generated by microgen. DO NOT EDIT.`（Go 标准的生成代码
标记）的文件，会在每次生成时整体重写；两者皆无的文件只在项目创建时写入一次，
此后绝不会被改动，可以放心编辑。

### 用户持有

- `cmd/main.go`
- `service/<service>/service.go`
- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`（标准库契约：路由直接注册到 mux；
  `customRouteDescriptions` 为调试路由清单提供条目）
- `model/<model>.go` —— GORM 生命周期钩子，只写一次
- `config/config.yaml` 与 `config/custom.go`（启用配置输出时）
- `README.md`
- 本地配置值与应用特有的集成包

### 生成器持有

- `.microgen/manifest.json` —— 所有权记录；下列工件全部登记其中
- `.ai/PROJECT_GUIDE.md` —— 随每个项目发布的 AI 代理指南
- `cmd/generated_*.go`（启用 interaction 时还有 `cmd/generated_interaction.go`）
- `endpoint/<service>/endpoints.go` 与 `generated_chain.go`
- `service/<service>/generated_repos.go`（启用模型输出时）
- `model/generated_*.go` 与 `repository/generated_*.go`
- `transport/<service>/` —— 生成的 HTTP 与 gRPC 处理器
- `client/<service>/demo.go` 与 `sdk/<service>sdk/`
- `pb/<service>/`（gRPC/proto 输出）、`test/`（测试输出）、`docs/` 与
  `sdk/typescript/`（契约输出）

不要把 `cmd/microgen` 下的模板或包当作运行时扩展 API。

### 开源协议

生成器写下的一切都属于你，包括生成器所有的文件：不要求声明、不要求署名、不产生任何回流到
本项目的义务，也不会写 `LICENSE` 文件，从而把选择权留给你。项目所 import 的框架仍然是
MIT。参见[开源协议](docs/licenses_zh.md)。

## 调用生成的服务

生成的 Go SDK 不需要你自己做任何代码生成步骤：

```go
client := helloservicesdk.New("http://localhost:8080")

resp, err := client.SayHello(ctx, idl.HelloRequest{Name: "world"})
var apiErr *helloservicesdk.APIError
if errors.As(err, &apiErr) {
	// apiErr.Status 是上游 HTTP 状态码；apiErr.Body 是响应体。
	// 在这里处理它——不要让上游的 404 变成你的 handler 的 404。
}
```

启用 gRPC 输出时，`NewGRPC(conn)` 在生成的 protobuf 桩上提供同一个客户端接口；
流式方法在流式客户端上。`client/<service>/demo.go` 是展示完整调用的可运行
main。TypeScript SDK 是一个无运行时依赖的 Fetch 客户端：

```ts
import { APIClient } from "./sdk/typescript/client";

const client = new APIClient("http://localhost:8080");
const response = await client.helloservice.SayHello({ name: "world" });
```

`APIClient` 的方法在非 2xx 响应时抛出 `APIError`，携带与 Go SDK 相同的状态码与
响应体。路由前缀对两者同样生效：以 `-prefix /api` 生成的服务从 `/api/...` 调用。

## 生成之后

```bash
cd <output>
go mod tidy
go test ./...
go run ./cmd
```

生成的测试在全新项目上即通过：每个尚未实现的方法都被**跳过**，跳过消息指名方法
与要实现的文件——整个套件把桩列表变成一张清单，方法一旦实现就自动变成真实断言。
在 `service/<name>/service.go` 中实现方法，然后阅读[依据契约生成前端]
(#依据契约生成前端)（如果前端会消费该 API）。

阅读生成的 `README.md`，检查复制进来的源契约，在对外暴露服务之前实现
业务方法。

## 生成器保证

- 生成的 Go 文件在写入前完成格式化。
- 模板或格式化错误不会留下写了一半的输出文件。
- 生成文本使用确定性的尾部空白与最终换行。
- 外部项目使用 v2 module 路径，不会收到非法的本地 `replace` 指令。
- 数据库内省不会改动源 schema。

生成器变更只有在夹具测试、重复生成检查、以及外部 `go mod tidy` 加
`go test ./...` 冒烟测试全部通过后才会合入。
