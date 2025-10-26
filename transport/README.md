# Transport 传输层模块

传输层模块负责处理不同通信协议下的数据传输，支持 HTTP 和 gRPC 两种主流协议。

## 核心组件

### HTTP 传输层

- **http/client/** - HTTP 客户端实现

  - `client.go` - HTTP 客户端核心逻辑
  - `encode_decode.go` - 请求/响应编解码
  - `options.go` - 客户端配置选项
  - `request_func.go` - 请求处理函数
  - `response_func.go` - 响应处理函数
  - `finalizer_func.go` - 最终化处理函数

- **http/server/** - HTTP 服务端实现
  - `server.go` - HTTP 服务器核心逻辑
  - `encode_decode.go` - 请求/响应编解码
  - `options.go` - 服务器配置选项
  - `intercepting_writer.go` - 拦截写入器
  - `request_func.go` - 请求处理函数
  - `response_func.go` - 响应处理函数
  - `finalizer_func.go` - 最终化处理函数

### gRPC 传输层

- **grpc/client/** - gRPC 客户端实现
- **grpc/server/** - gRPC 服务端实现

### 通用组件

- **error_handler.go** - 统一错误处理机制
- **context.go** - 上下文管理

## 快速使用

### HTTP 服务端示例

```go
import (
    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/dreamsxin/go-kit/transport/http/server"
)

// 创建端点
var ep endpoint.Endpoint = func(ctx context.Context, request interface{}) (interface{}, error) {
    return map[string]string{"message": "Hello, World!"}, nil
}

// 创建HTTP处理器
handler := server.NewServer(
    ep,
    decodeRequest,
    encodeResponse,
    server.ServerErrorEncoder(errorEncoder),
)

// 启动HTTP服务
http.Handle("/api", handler)
http.ListenAndServe(":8080", nil)
```

### HTTP 客户端示例

```go
import "github.com/dreamsxin/go-kit/transport/http/client"

// 创建HTTP客户端
httpClient := client.New(
    ep,
    encodeRequest,
    decodeResponse,
    client.ClientErrorDecoder(errorDecoder),
)
```

## API 参考

### HTTP 服务器选项

- `ServerBefore` - 请求前处理钩子
- `ServerAfter` - 响应后处理钩子
- `ServerErrorEncoder` - 错误编码器
- `ServerErrorHandler` - 错误处理器
- `ServerFinalizer` - 最终化函数

### HTTP 客户端选项

- `ClientBefore` - 请求前处理钩子
- `ClientAfter` - 响应后处理钩子
- `ClientErrorDecoder` - 错误解码器

## 最佳实践

1. **错误处理**：实现统一的错误编码/解码逻辑
2. **请求验证**：在解码器中验证请求参数
3. **响应包装**：统一响应格式和状态码处理
4. **超时控制**：合理设置请求超时时间
5. **重试机制**：实现客户端重试逻辑
