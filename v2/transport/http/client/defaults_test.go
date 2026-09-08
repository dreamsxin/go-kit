package client

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

// The default client is the one thing in this package a user gets without asking, so
// it is the one thing worth pinning: what it is not, and what it deliberately does not
// decide.

// TestDefaultClientIsNotTheStandardLibraryDefault is the regression that matters.
// http.DefaultClient is reachable by every library in the process, so a dependency
// that sets a timeout on it or swaps its Transport would silently change these calls.
func TestDefaultClientIsNotTheStandardLibraryDefault(t *testing.T) {
	if DefaultClient() == http.DefaultClient {
		t.Fatal("the default client is http.DefaultClient; another library's changes to it would reach these calls")
	}
	if DefaultClient().Transport == http.DefaultTransport {
		t.Fatal("the default transport is http.DefaultTransport, which is shared and allows 2 idle connections per host")
	}
}

// TestDefaultTransportPoolIsSizedForAService pins the number the audit found: two idle
// connections per host is right for a one-shot tool and wrong for a service calling
// the same upstream on every request.
func TestDefaultTransportPoolIsSizedForAService(t *testing.T) {
	transport, ok := DefaultClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default transport is %T, want *http.Transport", DefaultClient().Transport)
	}
	if transport.MaxIdleConnsPerHost != DefaultMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, DefaultMaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != DefaultIdleConnTimeout {
		t.Errorf("IdleConnTimeout = %s, want %s", transport.IdleConnTimeout, DefaultIdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != DefaultTLSHandshakeTimeout {
		t.Errorf("TLSHandshakeTimeout = %s, want %s", transport.TLSHandshakeTimeout, DefaultTLSHandshakeTimeout)
	}
	if transport.DialContext == nil {
		t.Error("DialContext is nil, so a host that does not answer is not bounded by a dial timeout")
	}
}

// TestDefaultClientHasNoInventedTimeout states the other half. A Timeout here would
// cap every call in the process at a number this package chose, invisibly from the
// call site; the deadline belongs to the caller's context.
func TestDefaultClientHasNoInventedTimeout(t *testing.T) {
	if DefaultClient().Timeout != 0 {
		t.Errorf("Timeout = %s, want 0: a deadline is the caller's, not this package's", DefaultClient().Timeout)
	}
}

func TestNewTransportReturnsAFreshTransportEachTime(t *testing.T) {
	first, second := NewTransport(), NewTransport()
	if first == second {
		t.Fatal("NewTransport returned the same transport twice; a caller tuning one would change the other")
	}
	if first == DefaultClient().Transport {
		t.Fatal("NewTransport returned the default client's transport; mutating it would change calls that did not opt in")
	}
}

// TestSetClientNilFallsBackToOurDefault: a nil client is a mistake, and the recovery
// should not quietly hand the caller the process-wide default.
func TestSetClientNilFallsBackToOurDefault(t *testing.T) {
	target, err := url.Parse("http://example.invalid/things")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	built := NewClient(http.MethodGet, target,
		func(context.Context, *http.Request, any) (*http.Request, error) { return nil, nil },
		func(context.Context, *http.Response) (any, error) { return nil, nil },
		SetClient(nil),
	)
	if built.client != DefaultClient() {
		t.Errorf("client = %v, want the package default", built.client)
	}
}

func TestNewClientUsesTheDefaultClientWhenGivenNoOption(t *testing.T) {
	target, err := url.Parse("http://example.invalid/things")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	built := NewClient(http.MethodGet, target,
		func(context.Context, *http.Request, any) (*http.Request, error) { return nil, nil },
		func(context.Context, *http.Response) (any, error) { return nil, nil },
	)
	if built.client != DefaultClient() {
		t.Errorf("client = %v, want the package default", built.client)
	}
}
