# resource/rest (HTTP binding)

English | [简体中文](README_zh.md)

`rest` mounts a `resource.Collection` over HTTP. It is a binding, not a second
stack: a route is a value — pattern, endpoint, decoder — and the application
decides what surrounds it.

```go
routes, err := rest.Routes(products)
if err != nil {
    return err
}
for _, route := range routes {
    kit.HandleEndpoint(http, route.Pattern, route.Endpoint, route.Decode)
}
```

A plain `*http.ServeMux` works the same way:

```go
for _, route := range routes {
    mux.Handle(route.Pattern, route.Handler())
}
```

The two differ in what surrounds the endpoint, not in what it answers: the kit
path keeps the component's endpoint middleware, recorders and JSON server
options, while a bare handler has only what `net/http` gives it. The body,
status and error code are identical either way, because both encode through the
transport's JSON encoder and error encoder — there is a test that compares them
byte for byte.

## Route table

| Pattern | Action | Success |
| --- | --- | --- |
| `GET {prefix}/{name}` | list | 200, a page |
| `POST {prefix}/{name}` | create | 201, the record, `Location` |
| `GET {prefix}/{name}:aggregate` | aggregate | 200, rows |
| `GET {prefix}/{name}/{id}` | get | 200, the record |
| `PATCH {prefix}/{name}/{id}` | update | 200, the record |
| `DELETE {prefix}/{name}/{id}` | destroy | 204, no body |

The aggregate route is `{name}:aggregate`, not `{name}/aggregate`: a wildcard
fills a whole segment, so `/products/aggregate` would shadow the record whose
identifier is `aggregate`. A suffix cannot.

Collection names are restricted to letters, digits, `-`, `_`, `~` and `.`, which
is what makes a name safe in a pattern and unambiguous next to that suffix.

## Parameters

| Parameter | Routes | Meaning |
| --- | --- | --- |
| `filter` | list, aggregate | the dialect `resource.ParseFilter` reads; several are ANDed |
| `sort` | list | `-field,other` — minus descending, plus accepted |
| `fields` | list, get | projection; `fields=` (empty) is refused, not widened |
| `append` | list | relations to expand; no repository in this version expands them |
| `page`, `size` | list | parsed by the transport's own `ParsePage` |
| `function`, `field`, `group_by` | aggregate | the figure and its groups |

A parameter a route does not read is refused with
`rest.parameter_not_allowed`, never ignored: a filter that was dropped answers a
broader question, and a broader answer is indistinguishable from an
authorization failure at the call site.

A column restriction holds on every answer, not only on a list: a write returns
the record projected, and an aggregate over a restricted column is refused with
`rest.aggregate_field_not_allowed` — summing a column or grouping by it reveals
what listing it would.

## Bodies

`POST` and `PATCH` read a JSON object. Every name must be a field of the schema
(`resource.unknown_field`) and every value must fit its type
(`resource.value_type_mismatch`); the body is decoded with `UseNumber`, so an
integer wider than 2^53 arrives exactly as sent. A field outside the write mask
is `resource.field_not_writable` (403) rather than dropped.

An empty body is an empty record: a create relies on the schema's defaults and a
patch changes nothing. A body that is not an object is
`rest.body_not_an_object`.

## The layer seam

```go
rest.WithDefaults(resource.Params{Sort: []resource.Sort{resource.Asc("name")}})
rest.WithLayer(func(ctx context.Context, action rest.Action) (resource.Params, error) {
    return resource.Params{Filter: ownerFilter(ctx)}, nil
})
```

A request is three layers merged in one order: the resource's defaults, what the
client asked for, then every installed authority. `resource.MergeParams` gives
each key one rule and every rule narrows — a filter only conjoins, a projection
only intersects, a page size only shrinks. So a default can be overridden by a
request, and a policy cannot.

`WithLayer` is where authorization plugs in: the layer sees the context and the
action, including `destroy`, and returns constraints rather than a yes/no.

## What this package deliberately does not do

- **No `PUT`.** A replace needs the schema to state which fields are required;
  it does not, and a replace that invents a zero value for an omitted column is
  a data loss bug with a 200 on top.
- **No composite keys in a path.** One segment names one value. `Routes` refuses
  a collection whose primary key has several fields instead of addressing half a
  record.
- **No authorization.** It offers the seam and stays out of the decision; the
  policy layer owns that.
- **No cursor pagination, no association expansion.** Both are `resource`
  decisions first; this package will not invent a spelling for them.
- **No middleware option.** Middleware belongs to whatever mounts the route. An
  option here would be silently ignored on the kit path, which is worse than
  absent.
