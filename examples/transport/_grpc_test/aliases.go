package test

// aliases.go — server.go 中使用了未导出的小写函数名，
// 这里统一做别名桥接，保持对外导出接口不变。

// ── 编解码 ───────────────────────────────────────────────

var (
	decodeRequest  = DecodeRequest
	encodeResponse = EncodeResponse
)

// ── Server Before / After hooks ──────────────────────────

var (
	extractCorrelationID        = ExtractCorrelationID
	displayServerRequestHeaders = DisplayServerRequestHeaders

	injectResponseHeader         = InjectResponseHeader
	injectResponseTrailer        = InjectResponseTrailer
	injectConsumedCorrelationID  = InjectConsumedCorrelationID
	displayServerResponseHeaders = DisplayServerResponseHeaders
	displayServerResponseTrailers = DisplayServerResponseTrailers
)
