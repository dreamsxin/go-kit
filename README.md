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
github.com/dreamsxin/go-kit/v2@v2.1.0
```

`v2.1.0` is a documented one-time SemVer exception and is not source compatible
with `v2.0.0`. Existing v2 users must read the
[migration guide](v2/MIGRATION.md) before upgrading. Normal compatibility rules
resume from `v2.1.0`.

## Start Here

Install the core framework and generator:

```bash
go get github.com/dreamsxin/go-kit/v2@v2.1.0
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.1.0
```

Use the [v2 README](v2/README.md) for installation, component selection,
generation, examples, and development commands.

## Documentation

- [v2 README](v2/README.md): user entry point
- [Architecture](v2/ARCHITECTURE.md): package boundaries and extension model
- [microgen](v2/MICROGEN.md): generator behavior and generated ownership
- [Migration](v2/MIGRATION.md): v1 and v2.0.0 upgrade guidance
- [Production](v2/PRODUCTION.md): runtime, security, and observability guidance
- [Release policy](v2/RELEASE.md): compatibility and release process

## v1 Archive

v1 is no longer maintained on `main`. Its complete source and documentation
remain available through the immutable legacy tags; `v1.6.0` is the final v1
release. Existing users can [browse the archived source](https://github.com/dreamsxin/go-kit/tree/v1.6.0)
or continue to pin it:

```bash
go get github.com/dreamsxin/go-kit@v1.6.0
```

New development should use v2.

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
