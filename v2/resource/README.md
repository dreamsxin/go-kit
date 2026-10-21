# resource

English | [简体中文](README_zh.md)

The `resource` package is the query vocabulary of a resource route: a filter
tree, the field metadata it is validated against, and the canonical form a
condition is stored in.

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

## Deliberate non-decisions

- **No runtime operator registration.** The set is closed; a condition it cannot
  express is a schema problem.
- **No SQL here.** A filter is translated by whoever owns the storage — the
  application's own adapter, or a separate package — so this one does not pull a
  driver into every consumer.
- **No cursor pagination yet.** Cursor semantics are keyset semantics, and a
  generic one would force a sort-key discipline this layer does not own.
- **No association filters.** `posts.comments` is not a filter path in the first
  version; relations belong to a projection parameter.
