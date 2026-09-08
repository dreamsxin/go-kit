package client

import (
	"net"
	"net/http"
	"time"
)

// The client this package uses when the caller supplies none.
//
// It used to be http.DefaultClient, which is wrong for a service in two ways that
// have nothing to do with policy.
//
// The first is sharing. http.DefaultClient is a package-level variable every library
// in the process can reach; a dependency that sets a timeout on it, or replaces its
// Transport, changes calls this package makes. A service's outbound calls should not
// depend on what else is linked into the binary.
//
// The second is the pool. http.DefaultTransport allows two idle connections per host
// (MaxIdleConnsPerHost defaults to 2). A service calling one upstream hard therefore
// closes and re-dials constantly, which shows up as latency nobody can explain from
// the application code. That default is right for a command-line tool talking to many
// hosts once, and wrong for a service talking to a few hosts continuously.
//
// What this package still does not decide is the deadline. There is no Timeout here,
// because a deadline belongs to the call: endpoint.Builder.WithTimeout,
// NewJSONClientWithTimeout, or a context the caller derives. A Timeout set here would
// silently cap every call in the process at a number this package invented, and would
// be invisible at the call site — see PRODUCTION.md.

// Connection pool and handshake defaults for the transport this package builds.
//
// They describe a service that talks to a small number of upstreams continuously.
// A deployment that knows better replaces the whole client with SetClient.
const (
	// DefaultMaxIdleConnsPerHost keeps enough idle connections for a service that
	// calls the same upstream on every request, instead of http.DefaultTransport's 2.
	DefaultMaxIdleConnsPerHost = 100
	// DefaultMaxIdleConns bounds the pool across all hosts.
	DefaultMaxIdleConns = 100
	// DefaultIdleConnTimeout is how long an unused connection is kept. It is shorter
	// than the idle timeout of most servers and proxies, so this side closes first
	// and a request does not race a server-side close.
	DefaultIdleConnTimeout = 60 * time.Second
	// DefaultDialTimeout bounds connecting to a host that is not answering.
	DefaultDialTimeout = 5 * time.Second
	// DefaultTLSHandshakeTimeout bounds a handshake that stalls after the connection.
	DefaultTLSHandshakeTimeout = 10 * time.Second
	// DefaultExpectContinueTimeout bounds waiting for a 100 Continue that may never
	// arrive.
	DefaultExpectContinueTimeout = time.Second
)

// NewTransport returns the *http.Transport this package uses by default.
//
// It is exported because a deployment that wants the pool defaults but needs one
// thing different — a proxy, a TLS config, HTTP/2 disabled — should start from this
// rather than from http.DefaultTransport, which is shared with every other library in
// the process.
func NewTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   DefaultDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          DefaultMaxIdleConns,
		MaxIdleConnsPerHost:   DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		TLSHandshakeTimeout:   DefaultTLSHandshakeTimeout,
		ExpectContinueTimeout: DefaultExpectContinueTimeout,
	}
}

// defaultClient is built once and shared by every Client in this process that did not
// get one from its caller. Sharing a client is the point — it is what makes the
// connection pool a pool — and it is this package's own, not the standard library's
// global.
//
// Stable: httpclient.pool-is-ours — a client this package builds does not use http.DefaultClient, so another library's changes to the process-wide default cannot reach these calls, and the connection pool is sized for a service rather than for a one-shot tool.
// Covered by: TestDefaultClientIsNotTheStandardLibraryDefault, TestSetClientNilFallsBackToOurDefault
//
// Stable: httpclient.no-invented-deadline — the default client sets no Timeout, because a deadline belongs to the call; pass one through the context or the endpoint builder.
// Covered by: TestDefaultClientHasNoInventedTimeout
var defaultClient = &http.Client{Transport: NewTransport()}

// DefaultClient returns the client this package uses when the caller supplies none.
//
// It is exported so a caller can reuse the same pool for calls it makes outside this
// package, and so a test can assert what it is.
func DefaultClient() *http.Client { return defaultClient }
