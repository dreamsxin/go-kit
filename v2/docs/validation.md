# Validation

English | [简体中文](validation_zh.md)

Two validation systems ship in the framework, and they answer different
questions. This chapter says which to reach for, what each produces on the
wire, and where the vocabulary is shared.

## The short version

| System | Package | Question it answers | Failure code |
| --- | --- | --- | --- |
| Struct tags | `validation` | does this decoded request satisfy the rules its fields declare? | `validation.<field>.<rule>` |
| Interface | `endpoint` | does this request, as a whole, make sense to the business? | `bad_request.validation` |

Both classify as `invalid_argument`, so both map to 400 over HTTP and
`InvalidArgument` over gRPC. A client never has to distinguish them; a log
reader does, and the code names the field.

## Tag validation

```go
type CreateUserRequest struct {
    Status string `json:"status" validate:"enum=active|suspended,default=active"`
    Age    int    `json:"age"    validate:"min=0,max=150"`
    Name   string `json:"name"   validate:"minLen=1,maxLen=64"`
}
```

`validation.Middleware()` walks the request's `validate` tags and refuses a
value that breaks one. The rule set is deliberately small — `enum`, `min`,
`max`, `minLen`, `maxLen` — because every rule maps one-to-one onto a JSON
Schema keyword: one tag drives the generated contract, the form a frontend
builds from it, and the check the service runs. A `pattern` rule is
deliberately absent: Go struct tags cannot carry backslashes reliably, and
the patterns a form needs are full of them.

`default=` is the contract's annotation for what a form pre-fills, and two
opt-in mechanisms honor it on the server: `validation.Defaults(&req)` fills
every zero-valued field carrying `default=` (a zero value and an absent
value are the same thing — JSON cannot send an unset non-pointer field), and
`validation.DefaultsMiddleware()` does the same as endpoint middleware,
filling a decoded copy when the route hands the chain a value. Wire the
middleware before `validation.Middleware()`; a generated transport fills in
its decoder, before checking. `Validate` itself never fills — checking and
mutating stay separate steps.

A nil pointer field is absent — constraints do not run on it — because
required-ness is the field's own business: a non-pointer field is already
required in the generated contract.

Wire it with `kit.WithEndpointMiddleware(validation.Middleware())`, or rely
on the typed JSON routes and generated transports, which validate on their
own.

## Interface validation

When a rule needs more than one field — "end must be after start", "exactly
one of these" — the request implements `endpoint.Validatable`:

```go
func (r CreateBookingRequest) Validate() error {
    if !r.End.After(r.Start) {
        return endpoint.NewValidationError("end", "must be after start")
    }
    return nil
}

ep := endpoint.NewBuilder(createBooking).
    WithValidation().
    Build()
```

`endpoint.ValidationMiddleware()` calls `Validate()` before the endpoint
runs and collects `endpoint.ValidationError` field errors. The transport
encodes the failure as 400 with code `bad_request.validation` and the field
list in the error body.

## Which one, when

- Prefer **tags** for single-field shape rules that the contract should
  advertise: enums, ranges, lengths. The generated OpenAPI and JSON Schema
  carry them to the client before it sends anything.
- Prefer the **interface** for cross-field and business-adjacent rules the
  contract cannot express.
- The two compose: tags catch the shape, the interface catches the meaning,
  and both run as endpoint middleware in either order.
- The code namespaces are deliberately separate — a tag failure names the
  rule it broke, an interface failure reports through
  `bad_request.validation` — while both classify as `invalid_argument` and
  land as 400. Unifying the codes would withdraw a documented stable
  promise for a cosmetic gain; the distinction is the documented contract.

See the [`validation` package](../validation/validation.go) godoc for the
exact tag grammar and [Error handling](errors.md) for how the codes reach
each transport.
