// Package client adapts remote HTTP APIs into endpoints.
//
// NewClient returns an endpoint that encodes a request value into an outbound
// HTTP request, executes it with the caller's http.Client, and decodes the
// response; NewJSONClient provides the common JSON shape with a bounded
// success-response read. Behavior is extended through options: Before and
// After RequestFuncs, ResponseFuncs such as buffering, and FinalizerFunc for
// transport-level error classification.
//
// Pair this package with the service-discovery packages under
// [github.com/dreamsxin/go-kit/v2/sd] to balance and retry calls across
// discovered instances.
package client
