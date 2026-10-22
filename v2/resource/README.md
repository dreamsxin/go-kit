# resource

English | [简体中文](README_zh.md)

The `resource` package is the query vocabulary of a resource route: a filter
tree, the field metadata it is validated against, the canonical form a condition
is stored in, and the repository contract those three are handed to.

It exists because a condition written as a string cannot be narrowed by a
permission policy, compared in an audit log, or replayed from a bug report.

## Why a value, not a query string

A filter that is text belongs to the database driver that parses it. Nothing
between the request and that driver can add a condition — a policy that has to
restrict a row can only say yes or no, because it cannot reach into the string.

A filter that is a tree belongs to the framework, and then:

- a policy constraint is `resource.And(policy, client)` — the client's condition
  survives, the policy's is added, and neither can widen the other;
- an audit record stores `{"op":"and","children":[...]}`, so two queries can be
  compared by reading them;
- a rejected request names the operand that was wrong, `children[1].children[0]`,
  instead of quoting a string nobody can point at.

## The tree

```go
type Filter struct {
    Op       Op       // eq, ne, in, nin, gt, gte, lt, lte, like, prefix,
                      // dateRange, empty, notEmpty, and, or, not
    Field    string   // a comparison applies to one field
    Value    any      // its operand
    Children []Filter // a connective's operands
}
```

`Op` is a closed set. An application cannot register an operator: an operator is
a promise every repository adapter has to keep, so a custom one would be a query
fragment the policy layer cannot reason about and the audit log cannot replay.

The zero `Filter` matches every record — it is the absence of a condition, not a
condition that was lost. That distinction is the whole safety story:

| call | behaviour | why |
| --- | --- | --- |
| `And(zero, x)` | `x` | a condition that matches everything adds nothing to a conjunction |
| `And(a, b)` | `and` of both, in order | flattening, deduplicating or merging by field is how a constraint disappears |
| `Or(zero, x)` | kept, and rejected by `Validate` | `or`'s identity is "match nothing", so dropping an empty operand would turn it into "match everything" |
| `Not(zero)` | rejected | "not everything" is not a condition someone meant to write |

`And` performs exactly one simplification: dropping zero operands.

## The dialect clients send

One JSON object. Keys are field names or the connectives `$and`, `$or`, `$not`.
Several operators on one field are a conjunction.

```json
{"status": "active"}
{"age": {"$gte": 18, "$lt": 65}}
{"$or": [{"ownerId": 7}, {"shared": true}]}
{"id": {"$in": [3, 4]}, "publishedAt": {"$dateRange": ["2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"]}}
{"deletedAt": {"$empty": true}}
```

| operator | applies to | operand |
| --- | --- | --- |
| `$eq`, `$ne` | every type except an opaque document | one value |
| `$in`, `$nin` | every type | a non-empty list |
| `$gt`, `$gte`, `$lt`, `$lte` | string, int, float, time | one ordered value |
| `$like`, `$prefix` | string | a literal; `%` and `_` are characters, not wildcards |
| `$dateRange` | time | `[from, to)`, half-open so adjacent ranges do not both match a boundary |
| `$empty`, `$notEmpty` | every type | `true` or nothing |

`ParseFilter` accepts the shapes a caller actually holds: a `json.RawMessage` from
a request body, a `string` from a query parameter (including a string that carries
the document as text), a `map[string]any` from a decoded tool call, or `nil` for
"no filter".

Keys are visited in sorted order, so one document always produces one tree. Two
audit records for the same query differ only when the query did.

## Validation

`Filter.Validate(schema)` is the boundary where text becomes a condition:

- a field the resource does not declare is `filter.unknown_field`, never a
  condition that is quietly dropped;
- a field declared `Filterable: false` is `filter.field_not_filterable`;
- an operator that means nothing on the field's type is
  `filter.operator_not_allowed` — `$like` on an int, `$gt` on a bool,
  `$dateRange` on a string;
- an operand that is not the field's type is `filter.type_mismatch` — a fraction
  for an int, prose for a time, one bad element of an `$in` list.

Numbers arrive as `json.Number`, so an identifier wider than 2^53 is exact.

A tree is limited to 32 levels and 256 nodes, whether it was parsed or built in
Go: a request that will happily build a thousand-node condition is a
denial-of-service primitive, and an unbounded depth is a stack overflow behind an
authentication check.

## What a filter means

`Matcher` is the executable definition, and it uses SQL's three-valued logic:

| condition | record without the field |
| --- | --- |
| `name == "ling"` | unknown |
| `name != "ling"` | unknown |
| `not(name == "ling")` | unknown |
| `empty(name)` | true |

A record is returned only when the answer is true. The third row is why the
distinction is implemented at all: with two values, "everything except ling"
would also return every record that has no name, which is not what the query
says and not what a database would answer.

Values are normalized before they are compared — `int64` from a driver,
`json.Number` from a decoded request and `int` from application code all compare
equal to `3`. An adapter that skips that step answers a query about 3 with no
rows.

## A request is three layers

```go
type Params struct {
    Filter   Filter
    Sort     []Sort
    Fields   []string   // nil: every readable field; empty: none
    Appends  []string
    Window   Window
    Writable WriteMask
}
```

`MergeParams(defaults, client, policy)` applies the layers in order. Each key has
one rule, and every rule narrows:

| key | rule | why |
| --- | --- | --- |
| `Filter` | `and` | a client adds conditions; it cannot remove the policy's |
| `Fields` | intersect | a client may ask for fewer columns, never for more |
| `Appends` | union | expanding a relation is additive and carries no authority |
| `Sort` | overwrite | ordering has no security meaning; the last layer decides |
| `Window` | smallest size | page size is the only dimension of a denial of service |
| `Writable` | whitelists intersect, blacklists union | the only two operations that cannot grant a field a layer refused |

`nil` and the empty slice are different values here: `nil` is a layer with no
opinion, an empty non-nil slice is a layer that allows none. Two disjoint
whitelists therefore intersect to the empty one, which fails closed. A merge that
collapsed it to "unrestricted" would answer a refused request with every column,
which is why the ambiguous spelling — an empty literal written by hand — denies
everything instead of permitting everything. When the two whitelists are
projections, `MergeParams` reports `resource.fields_disjoint` rather than
returning records with no fields at all.

## A collection is the declaration

```go
collection, err := resource.NewCollection("products", schema, store, "id")
```

Name, schema, primary key fields, repository. It is a value, not a registry
entry, so two collections over one table with different schemas are two values an
application can hold at once — a narrower surface for one client and a wider one
for another.

Validation is the point of the constructor: a name that is not a path segment, a
primary key that is not a field, a nil repository each produce something a later
layer cannot repair. A value reached by a struct literal can be checked with
`Collection.Validate`, and every transport does.

`KeyFromText` turns one path segment into a key, in the field's own domain: an
integer key parses as an integer, and a value that does not parse is an
invalid-argument error naming the field, never a zero value — an identifier that
quietly became 0 addresses a different record.

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

The three single-record operations take a `Scope` — a key together with the
condition the caller was granted — and not a bare key. A key is an address, not a
permission: `Get(ctx, key)` answers "does this row exist", which is the one
question a caller outside its tenant must not be able to ask one identifier at a
time. The condition and the key are applied as one statement, because a check
followed by a write has a window in it, and the window is where an escalation
lives. A record the condition excludes is `NotFound`, never `PermissionDenied`:
"it exists but you may not see it" describes a table to somebody who is not
allowed to read it.

The interface belongs to this package and the implementation belongs to whoever
owns the storage, so a service that only needs HTTP does not build a database
driver. The obligations every implementation carries:

- every clause of a query takes effect; a filter is never dropped or relaxed;
- a scope's condition is applied with its key, and a record it excludes is
  reported the same way a missing one is;
- `Page.Total` counts the matches, not the records in the window;
- a reply is the schema's projection, so a write-only column cannot escape
  through a `SELECT *`;
- a missing record is `NotFound`, an empty key is `InvalidArgument`, a write
  outside the mask is `PermissionDenied`, and a field the schema does not declare
  is `InvalidArgument` — none of them is dropped silently.

`Page` serializes as `{"items","total","page","size","has_next"}`, the same shape
`transport/http.PageResult` writes. The two packages cannot import each other, so
a test in the transport compares the two payloads byte for byte and pins the
defaults and the page-size cap together.

## The reference store

`resource/memory` implements the interface over a slice and a key index. It is
not a production store — everything dies with the process — and that is not what
it is for: it proves the contract can be satisfied, and it gives every future
adapter an oracle. The same query run through it and through a SQL adapter must
select the same records, because it implements the semantics above.

It is also the fixture for testing a route or a policy without a database.

## Deliberate non-decisions

- **No runtime operator registration.** The set is closed; a condition it cannot
  express is a schema problem.
- **No SQL here.** A filter is translated by whoever owns the storage — the
  application's own adapter, or a separate package — so this one does not pull a
  driver into every consumer.
- **No cursor pagination yet.** Cursor semantics are keyset semantics, and a
  generic one would force a sort-key discipline this layer does not own.
- **No association filters or expansions.** `posts.comments` is neither a filter
  path nor an implemented projection in this version; `Appends` is carried and
  merged, and a repository that cannot expand it returns `Unimplemented` rather
  than records that silently lack the relation.
- **No silent strip on write.** A field outside the write mask is refused rather
  than discarded: a create that answers 200 having stored something else is
  worse than one that answers 403.
