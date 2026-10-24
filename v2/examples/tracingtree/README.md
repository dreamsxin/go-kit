# tracingtree

English | [简体中文](README_zh.md)

One request through the full observability stack, printed as a tree:

```text
trace 5ca10c5f1068fcf96b2a7d46848656e3
  └─ server GET /aggregate
    └─ internal GET /aggregate
      └─ client GET
```

The three levels are the framework's division of labour, made visible:

- `server GET /aggregate` — `oteladapter.HTTPServerMiddleware` around the
  whole server, one SERVER span per request named after the route the mux
  matched.
- `internal GET /aggregate` — `oteladapter.TracingMiddleware` around the
  endpoint, the span the business code and every middleware beneath it join.
- `client GET` — `oteladapter.NewRoundTripper` around the outbound
  transport, which also injects the trace context, so a traced upstream
  would continue the same tree.

Run it:

```bash
go run ./examples/tracingtree
```

The exporter that prints the tree is hand-written on purpose: providers,
exporters and their shutdown belong to the application, and twenty lines is
the smallest exporter that shows the shape. A deployment replaces it with
`oteladapter.Setup` pointed at a collector — see the
[observability chapter](../../docs/observability.md).
