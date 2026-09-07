# 开源协议

[English](licenses.md) | 简体中文

## 本项目

MIT。协议文本位于模块旁的 [`LICENSE.txt`](../LICENSE.txt) 以及仓库根目录——发布的模块是
`v2` 子目录，因此无论使用方从哪一侧 vendor，协议都紧邻代码。两份是同一个文件，并由门禁
保持一致。

## 生成的项目

`microgen` 写进你项目里的代码属于你。版权持有人对生成产物不附加任何条件：不要求声明、
不要求署名、不产生任何回流到本项目的义务，你可以按自己的选择为它授予任何协议。生成器
不写协议头、也不生成 `LICENSE` 文件，正是为了把这个决定留给你，并由
`TestGeneratedOutputCarriesNoLicenseNotice` 保持这一点。

你的生成项目所 import 的框架是另一回事，它仍然是 MIT：上面的授予针对生成器写下的文件，
而不是 `github.com/dreamsxin/go-kit/v2` 本身。

## 依赖的协议

`github.com/dreamsxin/go-kit/v2` 是单一模块，因此 `go get` 会把下列每条依赖都记录进你的
`go.mod`，无论你是否 import 需要它的那个包。真正链接进二进制的内容由你 import 的包决定，
即右侧那一列。

- `github.com/emicklei/proto` —— MIT —— `cmd/microgen`（解析 Protobuf）
- `github.com/go-sql-driver/mysql` —— MPL-2.0 —— `cmd/microgen`（数据库反射）
- `github.com/hashicorp/consul/api` —— MPL-2.0 —— `integrations/consul`
- `github.com/lib/pq` —— MIT —— `cmd/microgen`（数据库反射）
- `go.etcd.io/etcd/client/v3` —— Apache-2.0 —— `integrations/etcd`
- `go.opentelemetry.io/otel` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/metric` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/sdk` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/sdk/metric` —— Apache-2.0 —— `observability/otel`
- `go.opentelemetry.io/otel/trace` —— Apache-2.0 —— `observability/otel`
- `go.uber.org/zap` —— MIT —— `integrations/zap`
- `google.golang.org/genproto/googleapis/rpc` —— Apache-2.0 —— `integrations/grpc`
- `google.golang.org/grpc` —— Apache-2.0 —— `integrations/grpc`
- `google.golang.org/protobuf` —— BSD-3-Clause —— `integrations/grpc` 与生成的 Protobuf 代码
- `modernc.org/sqlite` —— BSD-3-Clause —— `cmd/microgen`（数据库反射）

其中两条是 MPL-2.0，与其余条目不属于同一类义务：作为库使用不对你自己的代码提出任何要求，
但对这些文件本身的修改必须公开。二者都无法从纯 HTTP 或纯 gRPC 的服务里被触达——
`github.com/hashicorp/consul/api` 随 `integrations/consul` 进来，
`github.com/go-sql-driver/mysql` 随生成器的数据库反射进来——而保证这一点成立的是
`TestPackageDependencyBoundaries`，不是习惯。

间接依赖不在此列出。它们随上述模块的版本变化，并归属于那些项目自己的声明；
`go mod download -json` 与 `go version -m <binary>` 会报告某次具体构建实际解析到的内容。

## 仅存在于仓库中的模块

`tools`、`tools/contractcheck` 与 `examples` 永不发布，这由
`TestOnlyOneModuleIsPublishable` 保证。它们的依赖——例如契约门禁运行的 OpenAPI 与
JSON Schema 校验器——不会到达框架的使用方。

## 由什么检查

`TestDependencyLicensesAreDocumented` 读取 `v2/go.mod` 的直接依赖，在模块缓存中定位每个
模块，从它自己的协议文件判定协议类型，并与上面的清单比对。新增依赖、移除依赖，或某个依赖
更换了协议，都会导致失败，直到这份文档写明为止。
