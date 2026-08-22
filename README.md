# go-kit

[![Go Version](https://img.shields.io/badge/go-1.25.8+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

English | [Simplified Chinese](README_zh.md)

`go-kit` is a component-oriented Go service framework built around one request
path:

```text
Service -> Endpoint -> Transport
```

## Current Release

The maintained product line is the independent v2 module under [`v2/`](v2/):

```text
github.com/dreamsxin/go-kit/v2@v2.4.3
```

`v2.4.3` is the current published release. It is backward
compatible: additive capabilities and behavioral fixes only. Per-release
changes are recorded in the [changelog](v2/CHANGELOG.md); upgrade notes live
in the [migration guide](v2/MIGRATION.md).

## Start Here

Install the core framework and generator:

```bash
go get github.com/dreamsxin/go-kit/v2@v2.4.3
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.5
```

Use the [v2 README](v2/README.md) for installation, component selection,
generation, examples, and development commands.

## Documentation

- [v2 README](v2/README.md): user entry point
- [Architecture](v2/ARCHITECTURE.md): package boundaries and extension model
- [microgen](v2/MICROGEN.md): generator behavior and generated ownership
- [Upgrade notes](v2/MIGRATION.md): upgrade actions between releases
- [Production](v2/PRODUCTION.md): runtime, security, and observability guidance
- [Release policy](v2/internal/docs/RELEASE.md): compatibility and release process

## Development

Run the core tests:

```bash
go -C v2 test ./...
```

Run the maintained multi-module verification suites:

```bash
go -C v2/tools run ./releaseverify -root .. -suites test,standalone,vet
```

## License

MIT. See [LICENSE.txt](LICENSE.txt).
