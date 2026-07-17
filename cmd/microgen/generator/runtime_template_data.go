package generator

import "github.com/dreamsxin/go-kit/cmd/microgen/ir"

type serviceTemplateData struct {
	Service             *serviceView
	IRService           *ir.Service
	UnaryMethods        []*ir.Method
	ServerStreamMethods []*ir.Method
	ClientStreamMethods []*ir.Method
	BidiStreamMethods   []*ir.Method
	Models              []*modelView
	WithModel           bool
	ImportPath          string
	Source              string
}

type serviceGeneratedReposTemplateData struct {
	Service    *serviceView
	Models     []*modelView
	WithModel  bool
	ImportPath string
}

type endpointTemplateData struct {
	Service      *serviceView
	IRService    *ir.Service
	UnaryMethods []*ir.Method
	ImportPath   string
	Source       string
}

type endpointGeneratedChainTemplateData struct {
	Service              *serviceView
	GeneratedMiddlewares []string
}

type endpointCustomChainTemplateData struct {
	Service *serviceView
}

type httpTransportTemplateData struct {
	Service      *serviceView
	IRService    *ir.Service
	UnaryMethods []*ir.Method
	ImportPath   string
	RoutePrefix  string
	Source       string
}

type grpcTransportTemplateData struct {
	Service             *serviceView
	IRService           *ir.Service
	UnaryMethods        []*ir.Method
	ServerStreamMethods []*ir.Method
	ClientStreamMethods []*ir.Method
	BidiStreamMethods   []*ir.Method
	ImportPath          string
	Source              string
}

type protoTemplateData struct {
	Service        *serviceView
	IRService      *ir.Service
	Messages       []protoMessage
	NeedsTimestamp bool
	NeedsDuration  bool
}

type serviceTestTemplateData struct {
	Service             *serviceView
	IRService           *ir.Service
	UnaryMethods        []*ir.Method
	ServerStreamMethods []*ir.Method
	ClientStreamMethods []*ir.Method
	BidiStreamMethods   []*ir.Method
	ImportPath          string
	Source              string
}

type clientTemplateData struct {
	Service             *serviceView
	IRService           *ir.Service
	UnaryMethods        []*ir.Method
	ServerStreamMethods []*ir.Method
	ClientStreamMethods []*ir.Method
	BidiStreamMethods   []*ir.Method
	ImportPath          string
	WithGRPC            bool
	RoutePrefix         string
	Source              string
}

type sdkTemplateData struct {
	Service             *serviceView
	IRService           *ir.Service
	UnaryMethods        []*ir.Method
	ServerStreamMethods []*ir.Method
	ClientStreamMethods []*ir.Method
	BidiStreamMethods   []*ir.Method
	ImportPath          string
	WithGRPC            bool
	Source              string
	RoutePrefix         string
}

type modelTemplateData struct {
	Model          *modelView
	ImportPath     string
	AddAuditFields bool
	NeedsTime      bool
	NeedsGorm      bool
}

type modelHooksTemplateData struct {
	Name string
}

type repositoryBaseTemplateData struct {
	ImportPath string
}

type repositoryTemplateData struct {
	Model      *modelView
	ImportPath string
}
