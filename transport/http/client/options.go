package client

type ClientOption func(*Client)

// SetClient overrides the default http.DefaultClient with a custom HTTPClient.
// Use this to configure TLS, timeouts, or a custom transport.
func SetClient(client HTTPClient) ClientOption {
	return func(c *Client) { c.client = client }
}

// ClientBefore adds RequestFunc hooks that run before the request is sent.
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

// BufferedStream controls how the response body is read.
// When true, the body is not closed automatically; the caller is responsible
// for closing it.  Use this for streaming responses.
func BufferedStream(buffered bool) ClientOption {
	return func(c *Client) { c.bufferedStream = buffered }
}
