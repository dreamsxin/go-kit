package consul

import (
	"strconv"

	"github.com/dreamsxin/go-kit/log"

	stdconsul "github.com/hashicorp/consul/api"
)

// 服务注册类
type Registrar struct {
	client       Client
	registration *stdconsul.AgentServiceRegistration
	logger       *log.Logger
}

type RegistrarOption func(*Registrar)

func IDRegistrarOptions(id string) RegistrarOption {
	return func(r *Registrar) {
		r.registration.ID = id
	}
}

func TagsRegistrarOptions(tags []string) RegistrarOption {
	return func(r *Registrar) {
		r.registration.Tags = tags
	}
}

func NamespaceRegistrarOptions(namespace string) RegistrarOption {
	return func(r *Registrar) {
		r.registration.Namespace = namespace
	}
}

func CheckRegistrarOptions(check *stdconsul.AgentServiceCheck) RegistrarOption {
	return func(r *Registrar) {
		r.registration.Check = check
	}
}

func NewRegistrar(client Client, logger *log.Logger, name string, address string, port int, options ...RegistrarOption) *Registrar {

	r := &Registrar{
		client: client,
		registration: &stdconsul.AgentServiceRegistration{
			Name:    name,
			Port:    port,
			Address: address,
		},
		logger: logger,
	}
	for _, option := range options {
		option(r)
	}
	if r.registration.ID == "" {
		r.registration.ID = r.registration.Name + "-" + r.registration.Address + "-" + strconv.Itoa(r.registration.Port)
	}
	return r
}

func (p *Registrar) Register() {
	if err := p.client.Register(p.registration); err != nil {
		p.logger.Sugar().Debugln("err", err)
	} else {
		p.logger.Sugar().Debugln("action", "register")
	}
}

func (p *Registrar) Deregister() {
	if err := p.client.Deregister(p.registration); err != nil {
		p.logger.Sugar().Debugln("err", err)
	} else {
		p.logger.Sugar().Debugln("action", "deregister")
	}
}
