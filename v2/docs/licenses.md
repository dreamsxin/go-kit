# Licensing

English | [简体中文](licenses_zh.md)

## This project

MIT. The text is in [`LICENSE.txt`](../LICENSE.txt) next to the module, and in the
repository root, because the published module is the `v2` subdirectory: a consumer
who vendors it gets the license beside the code either way. Both copies are the
same file, and a gate keeps them that way.

## Dependency licenses

`github.com/dreamsxin/go-kit/v2` is one module, so `go get` records every
requirement below in your `go.mod` whether or not you import the package that
needs it. What actually links into your binary is decided by the packages you
import, which is the column on the right.

- `github.com/emicklei/proto` — MIT — `cmd/microgen` (Protobuf parsing)
- `github.com/go-sql-driver/mysql` — MPL-2.0 — `cmd/microgen` (database introspection)
- `github.com/hashicorp/consul/api` — MPL-2.0 — `integrations/consul`
- `github.com/lib/pq` — MIT — `cmd/microgen` (database introspection)
- `go.etcd.io/etcd/client/v3` — Apache-2.0 — `integrations/etcd`
- `go.opentelemetry.io/otel` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/metric` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/sdk` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/sdk/metric` — Apache-2.0 — `observability/otel`
- `go.opentelemetry.io/otel/trace` — Apache-2.0 — `observability/otel`
- `go.uber.org/zap` — MIT — `integrations/zap`
- `google.golang.org/genproto/googleapis/rpc` — Apache-2.0 — `integrations/grpc`
- `google.golang.org/grpc` — Apache-2.0 — `integrations/grpc`
- `google.golang.org/protobuf` — BSD-3-Clause — `integrations/grpc` and generated Protobuf code
- `modernc.org/sqlite` — BSD-3-Clause — `cmd/microgen` (database introspection)

Two of these are MPL-2.0, which is not the same obligation class as the rest:
using the package as a library asks nothing of your own code, but modifications to
those files must be published. Neither is reachable from an HTTP-only or gRPC-only
service — `github.com/hashicorp/consul/api` arrives with `integrations/consul`, and
`github.com/go-sql-driver/mysql` with the generator's database introspection — and
`TestPackageDependencyBoundaries` is what keeps that true rather than a habit.

Indirect requirements are not listed. They change with the versions of the modules
above and belong to those projects' own notices; `go mod download -json` and
`go version -m <binary>` report what a given build actually resolved.

## Repository-only modules

`tools`, `tools/contractcheck` and `examples` are never published, which
`TestOnlyOneModuleIsPublishable` enforces. Their dependencies — the OpenAPI and
JSON Schema validators the contract gates run, among others — do not reach a
consumer of the framework.

## What checks this

`TestDependencyLicensesAreDocumented` reads the direct requirements from
`v2/go.mod`, finds each module in the module cache, classifies the license from its
own license file, and compares the result with the list above. A new dependency, a
removed one, or one that changes its license fails until this document says so.
