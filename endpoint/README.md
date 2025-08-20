# README

端点(Endpoint)管理核心模块，包含：

- endpoint.go - 端点接口定义与基础实现
- endpoint_cache.go - 端点缓存管理
- factory.go - 端点创建工厂
- middleware.go - 端点中间件框架
- circuitbreaker/ - 熔断器实现（gobreaker/hystrix）
- ratelimit/ - 限流实现（令牌桶算法）
