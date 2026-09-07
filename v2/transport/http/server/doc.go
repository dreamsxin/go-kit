// Package server adapts endpoints to inbound HTTP requests.
//
// NewServer wraps an endpoint with request decoding, response encoding, and
// error encoding, and returns a standard http.Handler; NewTypedJSONServer and
// the JSON helpers provide fully typed JSON assembly for concrete request and
// response types. Behavior is extended through options: Before and After
// RequestFuncs, FinalizerFunc for post-response observation, a bounded request
// body limit, and strict JSON decoding.
//
// The package also owns two things that belong to the transport rather than to a
// single handler. RouteRegistrar and DecorateRoutes install middleware at
// registration, which is the only place a handler and its route pattern are both in
// scope — http.Request.Pattern is set on the request a ServeMux dispatched, so
// middleware wrapped around the mux sees an empty route. Recorder,
// RecordingMiddleware, and AccessLogMiddleware report what the transport knows and
// the endpoint layer does not: the matched route, the status code, and the bytes
// written.
//
// Handlers compose with any net/http middleware; see the transport README for
// the role of this package in the Service -> Endpoint -> Transport path.
package server
