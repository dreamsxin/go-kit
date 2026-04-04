package client

type ClientOption func(*Client)

// ClientBefore adds RequestFunc hooks that run before the gRPC call is made.
func ClientBefore(before ...RequestFunc) ClientOption {
	return func(c *Client) { c.before = append(c.before, before...) }
}

// ClientAfter adds ResponseFunc hooks that run after a successful response.
func ClientAfter(after ...ResponseFunc) ClientOption {
	return func(c *Client) { c.after = append(c.after, after...) }
}

// ClientFinalizer adds FinalizerFunc hooks that always run at the end of a call.
func ClientFinalizer(f ...FinalizerFunc) ClientOption {
	return func(s *Client) { s.finalizer = append(s.finalizer, f...) }
}
