# README

- client.go: 封装 Consul API，提供服务注册、注销和实例查询功能
- instancer.go: 实现服务实例发现，支持标签过滤和健康检查
- registrar.go: 实现服务注册逻辑，支持自定义 ID、标签和健康检查
