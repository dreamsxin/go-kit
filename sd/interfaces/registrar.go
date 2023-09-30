package interfaces

// 服务注册接口
type Registrar interface {
	Register()
	Deregister()
}
