# microgen 用户指南

[English](MICROGEN.md) | 简体中文

`microgen` 从契约生成可直接运行的 Go 服务。它是新服务的推荐入口；运行时
各包仍可独立使用。

本指南对应 `microgen` `v0.2.9`，即独立版本化生成器模块，安装方式：

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.9
```

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
| `-openapi` | 生成 OpenAPI 3.1、JSON Schema、Swagger UI 与 TypeScript SDK |

以 `microgen -h` 为权威选项列表。

## 生成项目清单

每次完整生成都会写入 `.microgen/manifest.json`，schema 版本为
`microgen.project.v2`。它是生成项目的首要身份与再生成契约，记录：

- 源模式与 Go module 路径；
- 已启用的能力，以及配置模式、远程提供方与数据库驱动；
- 路由前缀、服务、生成的模型与生成中间件顺序；
- 所有生成器持有产物的规范化相对路径。

不要手工编辑清单。完整生成会在写完其余产物后刷新它。服务、模型与中间件
extend 操作最后刷新清单，因此不完整的写入不会声称完成了一次生成。

`microgen extend -check -out .` 校验清单 schema 与 module 路径，把声明的
产物与文件系统和所有权规则比对，报告缺失、未声明或类型不匹配的输出。
清单缺失或存在漂移时，extend 变更会被拒绝。

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

extend 模式当前只接受完整合并的 Go IDL 契约，不支持 Protobuf 输入。

先运行只读兼容性扫描：

```bash
microgen extend -check -out .
```

追加一个服务或模型，或一组受支持的中间件：

```bash
microgen extend \
  -idl full_combined.go \
  -out . \
  -append-service OrderService

microgen extend \
  -idl full_combined.go \
  -out . \
  -append-model Product

microgen extend \
  -idl full_combined.go \
  -out . \
  -append-middleware tracing,error-handling,metrics
```

extend 模式更新新文件与生成器持有的聚合文件。追加服务或模型时，若契约
输出已启用，还会刷新 OpenAPI、JSON Schema 与 TypeScript SDK。每次追加
操作都会刷新 `.microgen/manifest.json`。对存在清单漂移或缺少必需所有权
接缝的项目，它会拒绝执行。

## 文件所有权

### 用户持有

- `service/<service>/service.go`
- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`
- 本地配置值与应用特有的集成包

### 生成器持有

- `.microgen/manifest.json`
- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- 生成的客户端、Go/TypeScript SDK、OpenAPI/JSON Schema 资产、可选 MCP
  适配器与 protobuf 资产

不要把 `cmd/microgen` 下的模板或包当作运行时扩展 API。

## 生成之后

```bash
cd <output>
go mod tidy
go test ./...
go run ./cmd
```

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
