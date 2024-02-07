package client

type ClientOption func(*Client)

func ClientBefore(before ...RequestFunc) ClientOption {
	return func(c *Client) { c.before = append(c.before, before...) }
}

func ClientAfter(after ...ResponseFunc) ClientOption {
	return func(c *Client) { c.after = append(c.after, after...) }
}

func ClientFinalizer(f ...FinalizerFunc) ClientOption {
	return func(s *Client) { s.finalizer = append(s.finalizer, f...) }
}
