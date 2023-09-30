package events

// 服务实例发现事件
type Event struct {
	Instances []string
	Err       error
}
