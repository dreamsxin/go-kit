// Package server adapts endpoints to inbound HTTP requests.
//
// NewServer wraps an endpoint with request decoding, response encoding, and
// error encoding, and returns a standard http.Handler; NewTypedJSONServer and
// the JSON helpers provide fully typed JSON assembly for concrete request and
// response types. Behavior is extended through options: Before and After
// RequestFuncs, FinalizerFunc for post-response observation, a bounded request
// body limit, and strict JSON decoding.
//
// Handlers compose with any net/http middleware; see the transport README for
// the role of this package in the Service -> Endpoint -> Transport path.
package server
