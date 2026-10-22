# resource/rest (HTTP binding)

English | [简体中文](README_zh.md)

`rest` mounts a `resource.Collection` over HTTP. It is a binding, not a second
stack: a route is a value — pattern, endpoint, decoder — and the application
decides what surrounds it.

```go
// WithOpenAccess says anybody may use this collection; WithPolicy is the other
// answer, and a collection with neither is refused.
routes, err := rest.Routes(products, rest.WithOpenAccess())
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

## Who may use it

A collection is mounted with a decision, or it is refused:

```go
routes, err := rest.Routes(products, rest.WithPolicy(
    func(ctx context.Context, action rest.Action) (rest.Decision, error) {
        subject, ok := auth.Subject(ctx)
        if !ok {
            return rest.Decision{}, nil // the zero decision refuses
        }
        switch action {
        case rest.ActionList, rest.ActionGet, rest.ActionAggregate:
            return rest.Allow(resource.Params{Filter: ownedBy(subject)}), nil
        case rest.ActionCreate, rest.ActionUpdate:
            return rest.Allow(resource.Params{
                Filter:   ownedBy(subject),
                Writable: resource.WriteMask{Deny: []string{"owner_id"}},
            }), nil
        default:
            return rest.Deny("deletes are an administrator action"), nil
        }
    }))
```

`rest.WithOpenAccess()` is the other answer — everybody, everything. A collection
mounted with neither is refused by `Routes` (`rest.access_required`), and one
mounted with both is refused too (`rest.access_conflict`): a policy is a whitelist
and open access is its absence, so whichever won, the other line would not
describe what the process does.

Three properties are worth spelling out, because each is a way this kind of code
goes wrong:

- **The zero `Decision` refuses.** A policy somebody did not finish, a switch that
  falls through, a value that was never filled in — every one of them is silence,
  and silence means no. `rest.Allow(params)` is a statement somebody made on
  purpose.
- **Installing a policy closes the surface.** An action no policy grants is
  refused with `rest.action_not_allowed`, so what a client may do is what the
  policies allow rather than everything minus what somebody remembered to forbid.
  Several policies are several authorities — a tenant scope and a role — and every
  one must allow, because a union would let one authority overrule another's
  refusal.
- **An error is not a refusal.** A refusal is an answer the policy reached; an
  error says it could not answer, and it stops the request as a 500 rather than
  hiding behind a 403 for a day.

A policy sees the context and the action, `destroy` included, and it does not see
the record: the decision is a condition, not an inspection. That condition travels
with the key into storage, so `get`, `update` and `delete` on a record outside it
answer `resource.record_not_found` — the same answer as a record that does not
exist, because "it exists but you may not see it" describes a table to somebody
who may not read it. A create is checked the other way round: the record being
stored must satisfy the same condition, and a field the record does not carry
makes the condition unknown, which refuses with `rest.constraint_unsatisfied`.

The constraints merge like every other layer, after the resource's defaults and
after the client's parameters, and `resource.MergeParams` only narrows: a filter
conjoins, a projection intersects, a page size shrinks. So a default can be
overridden by a request, and a policy's condition cannot.

A condition that must never move is a field the schema marks unwritable: the write
mask refuses it on every adapter, which is what stops a patch from re-parenting a
record out of the scope it was granted. The condition is answered by the same
`WHERE` clause as the key, about the row as it stands — the framework does not
guess what a patch is about to do to it.

## What this package deliberately does not do

- **No `PUT`.** A replace needs the schema to state which fields are required;
  it does not, and a replace that invents a zero value for an omitted column is
  a data loss bug with a 200 on top.
- **No composite keys in a path.** One segment names one value. `Routes` refuses
  a collection whose primary key has several fields instead of addressing half a
  record.
- **No authentication.** It does not know who is calling: no identity, no roles,
  no sessions. What it owns is the shape of an answer — a decision that carries
  constraints, a default of refusal, and a condition that reaches storage.
- **No second authorization pass over the record.** A policy answers before the
  request runs, from the context and the action. A "fetch, then ask again" hook
  would be a decision that can contradict the first one, and the condition the
  first one returned is already what selects the record.
- **No cursor pagination, no association expansion.** Both are `resource`
  decisions first; this package will not invent a spelling for them.
- **No middleware option.** Middleware belongs to whatever mounts the route. An
  option here would be silently ignored on the kit path, which is worse than
  absent.
