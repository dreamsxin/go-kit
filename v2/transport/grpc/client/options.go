package client

type ClientOption func(*Client)

// ClientBefore adds RequestFunc hooks that run before the gRPC call is made.
func ClientBefore(before ...RequestFunc) ClientOption {
	return func(c *Client) {
		for _, hook := range before {
			if hook != nil {
				c.before = append(c.before, hook)
			}
		}
	}
}

// ClientAfter adds ResponseFunc hooks that run after a successful response.
func ClientAfter(after ...ResponseFunc) ClientOption {
	return func(c *Client) {
		for _, hook := range after {
			if hook != nil {
				c.after = append(c.after, hook)
			}
		}
	}
}

// ClientFinalizer adds FinalizerFunc hooks that always run at the end of a call.
func ClientFinalizer(f ...FinalizerFunc) ClientOption {
	return func(s *Client) {
		for _, hook := range f {
			if hook != nil {
				s.finalizer = append(s.finalizer, hook)
			}
		}
	}
}
