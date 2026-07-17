package client

import "net/http"

type ClientOption func(*Client)

// SetClient overrides the default http.DefaultClient with a custom HTTPClient.
// Use this to configure TLS, timeouts, or a custom transport.
func SetClient(client HTTPClient) ClientOption {
	return func(c *Client) {
		if client == nil {
			c.client = http.DefaultClient
			return
		}
		c.client = client
	}
}

// ClientBefore adds RequestFunc hooks that run before the request is sent.
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

// BufferedStream controls how the response body is read.
// When true, the body is not closed automatically; the caller is responsible
// for closing it.  Use this for streaming responses.
func BufferedStream(buffered bool) ClientOption {
	return func(c *Client) { c.bufferedStream = buffered }
}
