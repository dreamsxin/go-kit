# Resources

English | [简体中文](resources_zh.md)

A resource route is a table behind an HTTP pattern: list, create, get, update,
destroy, aggregate. The `resource` package is the vocabulary such a route
speaks — a filter tree, the field metadata it is validated against, the
merge that narrows a request, and the repository contract all of it is handed
to. This chapter is the path in; the exact contracts live in the
[`resource` README](../resource/README.md) and godoc.

## Why a tree, not a query string

A filter written as text belongs to the database driver that parses it.
Nothing between the request and that driver can narrow it: a policy that must
restrict a row can only say yes or no, because it cannot reach into the
string. A filter that is a value belongs to the framework:

- a policy constraint is `resource.And(policy, client)` — the client's
  condition survives, the policy's is added, neither can widen the other;
- an audit record stores the canonical tree, so two queries can be compared
  by reading them;
- a rejected request names the operand that was wrong, not a string nobody
  can point at.

Clients send one JSON object — `{"age": {"$gte": 18, "$lt": 65}}`, with
`$and`/`$or`/`$not`, `$in`, `$dateRange`, `$empty` — which
`resource.ParseFilter` turns into the tree. Keys are visited in sorted order,
so one document always produces one tree. `Filter.Validate(schema)` then
refuses what a schema does not declare: an unknown field, an unfilterable
one, an operator that means nothing on the field's type, an operand of the
wrong type. Nothing narrows silently.

## A request is three layers

Every request carries `resource.Params` — filter, sort, projection, appends,
window, write mask — merged from three layers: a default, the client, and a
policy. `MergeParams` applies each key with one rule, and every rule narrows:
filters are `and`ed, projections intersect, page size takes the smaller
value, whitelists intersect while blacklists union. A layer that wrote
nothing (`nil`) has no opinion; a layer that wrote an empty slice allows
none — the distinction is what makes two disjoint whitelists fail closed
instead of collapsing into "unrestricted".

## Mounting a collection

```go
collection, err := resource.NewCollection("products", schema, store, "id")
if err != nil {
    return err
}
routes, err := rest.Routes(collection, rest.WithPolicy(policy))
if err != nil {
    return err
}
for _, route := range routes {
    kit.HandleEndpoint(http, route.Pattern, route.Endpoint, route.Decode)
}
```

`resource.NewCollection` validates the declaration up front — a name that is
not a path segment, a primary key that is not a field, a nil repository each
fail at construction. `rest.Routes` turns it into route values: pattern,
endpoint, decoder. The same routes mount on a plain `*http.ServeMux` when
there is no `kit` component; what surrounds the endpoint differs, what it
answers does not.

The route table and the strict parameter reading (a parameter a route does
not read is refused, never ignored; a query string the URL parser cannot read
in full is refused whole) are in the
[`resource/rest` README](../resource/rest/README.md).

## The repository contract

```go
type Repository interface {
    List(ctx context.Context, query Query) (Page, error)
    Get(ctx context.Context, scope Scope) (Record, error)
    Create(ctx context.Context, record Record, mask WriteMask) (Record, error)
    Update(ctx context.Context, scope Scope, patch Record, mask WriteMask) (Record, error)
    Destroy(ctx context.Context, scope Scope) error
    Aggregate(ctx context.Context, query AggregateQuery) (AggregateResult, error)
}
```

Single-record operations take a `Scope` — a key together with the condition
the caller was granted — because a key is an address, not a permission, and a
check followed by a write has a window in it. A record the condition excludes
is `NotFound`, never `PermissionDenied`. `resource/memory` is the reference
implementation: not a production store, but the oracle every SQL adapter must
agree with, and the fixture for testing a route or policy without a database.

## Generated projects

`microgen -resource` (with `-db`) declares every generated table as a
`resource.Collection` and mounts its REST surface, so a generated service
answers the resource protocol on its tables. See the
[microgen guide](../MICROGEN.md) for the flag and the generated layout.

## What is deliberately absent

The operator set is closed — an operator is a promise every adapter must
keep, and a custom one is a query fragment the policy layer cannot reason
about. There is no SQL in this package (translation belongs to whoever owns
the storage), no cursor pagination yet, and no association filters: a
repository that cannot expand an `Appends` entry returns `Unimplemented`
rather than records that silently lack the relation. The full list with
reasons is in the [`resource` README](../resource/README.md).
