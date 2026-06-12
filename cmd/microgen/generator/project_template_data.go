package generator

import "github.com/dreamsxin/go-kit/cmd/microgen/ir"

type mainTemplateData struct {
	Project         *ir.Project
	Services        []*serviceView
	Models          []*modelView
	GormModels      []*modelView
	SvcRoutes       []SvcRoute
	ImportPath      string
	WithDB          bool
	DBDriver        string
	DBImportPkg     string
	DBOpenCall      string
	DBDefaultDSN    string
	WithConfig      bool
	WithGRPC        bool
	WithSwag        bool
	WithSkill       bool
	WithInteraction bool
}

type generatedRuntimeTemplateData struct {
	Project         *ir.Project
	GormModels      []*modelView
	WithDB          bool
	WithGRPC        bool
	WithSwag        bool
	WithSkill       bool
	WithInteraction bool
	SvcRoutes       []SvcRoute
	ImportPath      string
}

type generatedServicesTemplateData struct {
	Project    *ir.Project
	Services   []*serviceView
	GormModels []*modelView
	ImportPath string
	WithDB     bool
	WithConfig bool
}

type generatedRoutesTemplateData struct {
	Project        *ir.Project
	Services       []*serviceView
	SvcRoutes      []SvcRoute
	UnarySvcRoutes []SvcRoute
	ImportPath     string
	WithGRPC       bool
	RoutePrefix    string
}

type customRoutesTemplateData struct {
	ImportPath string
}

type configTemplateData struct {
	Services              []*serviceView
	DBDriver              string
	DBDefaultDSN          string
	DBConfigDSN           string
	WithGRPC              bool
	WithSwag              bool
	WithDB                bool
	ConfigMode            string
	RemoteProvider        string
	RemoteEnabledDefault  bool
	RemoteFallbackDefault bool
}

type readmeTemplateData struct {
	Project         *ir.Project
	IsProtoInput    bool
	WithSkill       bool
	WithInteraction bool
	WithConfig      bool
	ConfigMode      string
	RemoteProvider  string
}

type docsTemplateData struct {
	Services   []*serviceView
	LeftDelim  string
	RightDelim string
}

type skillTemplateData struct {
	Project    *ir.Project
	ImportPath string
}

type interactionTemplateData struct {
	Project    *ir.Project
	Services   []*serviceView
	ImportPath string
}

type aiProjectGuideTemplateData struct {
	Project        *ir.Project
	ImportPath     string
	WithConfig     bool
	WithDB         bool
	WithGRPC       bool
	WithSwag       bool
	WithSkill      bool
	WithInteraction bool
}

type goModTemplateData struct {
	ImportPath  string
	WithConfig  bool
	RootRelPath string
}
