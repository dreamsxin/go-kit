package endpoint

import "io"

// Factory creates an Endpoint for a given service instance address.
// It is used by EndpointCache and Endpointer to build endpoints on demand
// as service-discovery events arrive.
//
// The returned io.Closer (if non-nil) is called when the instance is removed
// from the cache, allowing the caller to release resources (e.g. close a
// gRPC connection).
type Factory func(instance string) (Endpoint, io.Closer, error)
