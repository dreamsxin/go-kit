package consul

import (
	stdconsul "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// 服务注册类
type Registrar struct {
	client       Client
	registration *stdconsul.AgentServiceRegistration
	logger       *zap.SugaredLogger
}

func NewRegistrar(client Client, logger *zap.SugaredLogger, id, name string, tags []string, address string, port int) *Registrar {

	return &Registrar{
		client: client,
		registration: &stdconsul.AgentServiceRegistration{
			ID:      id,
			Name:    name,
			Tags:    tags,
			Port:    port,
			Address: address,
		},
		logger: logger,
	}
}

func (p *Registrar) Register() {
	if err := p.client.Register(p.registration); err != nil {
		p.logger.Debugln("err", err)
	} else {
		p.logger.Debugln("action", "register")
	}
}

func (p *Registrar) Deregister() {
	if err := p.client.Deregister(p.registration); err != nil {
		p.logger.Debugln("err", err)
	} else {
		p.logger.Debugln("action", "deregister")
	}
}
