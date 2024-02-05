package client

type ClientOption func(*Client)

func SetClient(client HTTPClient) ClientOption {
	return func(c *Client) { c.client = client }
}

func ClientBefore(before ...RequestFunc) ClientOption {
	return func(c *Client) { c.before = append(c.before, before...) }
}

func ClientAfter(after ...ResponseFunc) ClientOption {
	return func(c *Client) { c.after = append(c.after, after...) }
}

func ClientFinalizer(f ...ResponseFunc) ClientOption {
	return func(s *Client) { s.finalizer = append(s.finalizer, f...) }
}

// 设置 body 读取方式为缓存流的方式，需自行关闭和清空
func BufferedStream(buffered bool) ClientOption {
	return func(c *Client) { c.bufferedStream = buffered }
}
