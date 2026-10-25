# Benchmarks

English | [简体中文](benchmarks_zh.md)

Two harnesses answer two different questions, and neither answer is a
marketing number: what the framework costs over the platform it sits on,
and how that cost compares with the frameworks a Go service realistically
picks between. Both are reproducible from this repository; the environment
section is not optional context, it is part of the result.

## Methodology

One JSON POST route (`{"name","count"}` in, `{"echo","double"}` out), served
by each contender from one process, driven by one keep-alive connection as
fast as the server answers, discarding bodies. `go test -bench` reports
ns/op, bytes/op and allocs/op; the numbers below are `-benchtime=2s` runs.
No TLS, no body larger than a paragraph, no connection churn — those
measure the transport and the client, not the framework, and a framework
choice rarely changes them.

## What the framework costs over the standard library

`examples/benchmark`, run in one process:

```bash
go -C v2/examples test ./benchmark -bench . -benchmem -benchtime=2s
```

| route | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `net/http` + `encoding/json` | 87,501 | 8,525 | 94 |
| kit (`kit.NewHTTP` + `kit.HandleJSON`) | 97,561 | 9,762 | 117 |

The framework's endpoint middleware chain, strict JSON decoding, validation
hooks and error classification cost about 11% latency and 23 allocations
per request over the raw platform. The route used here carries the
framework's validation tags, so the number includes work a bare handler
does not do.

## Against other frameworks

`tools/testdata/bench-thirdparty` — its own module on purpose, because a
comparison harness should not put four frameworks into the dependency
graph of the release:

```bash
cd v2/tools/testdata/bench-thirdparty
GOWORK=off go test -bench . -benchmem -benchtime=2s
```

Same route, same machine (Intel i5-10400F, Windows, loopback, go1.26.0):

| framework | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| stdlib `ServeMux` | 88,598 | 9,095 | 95 |
| **kit** | **97,881** | **10,215** | **117** |
| chi | 92,384 | 9,599 | 97 |
| gin | 93,148 | 9,341 | 95 |
| echo | 91,719 | 9,396 | 95 |

Read: kit costs 5–7% over the router frameworks on this route, all five
within ~11% of the platform. On loopback Windows the transport dominates;
absolute numbers move with the machine, the ordering is what reproduces.
Treat any framework benchmark that promises more precision than that as
marketing, including ours.

## What the numbers do not say

Per-request throughput is not why services pick a framework, or the
standard library would win every time. What the frameworks above differ in
is what surrounds the route: strict decoding, validation, error
classification and envelopes, OpenAPI generation, tracing and metrics
wiring, service discovery, retry with feedback, and MCP support. The
[feature chapters](index.md) are the honest comparison; this page exists
so the cost of those features is on the table too.
