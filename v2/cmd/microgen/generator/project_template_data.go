package generator

import "github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"

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
	WithOpenAPI     bool
	WithSkill       bool
	WithInteraction bool
}

type generatedRuntimeTemplateData struct {
	Project         *ir.Project
	GormModels      []*modelView
	WithDB          bool
	WithGRPC        bool
	WithOpenAPI     bool
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
	WithDB                bool
	ConfigMode            string
	RemoteProvider        string
	RemoteEnabledDefault  bool
	RemoteFallbackDefault bool
}

type readmeTemplateData struct {
	Project         *ir.Project
	IsProtoInput    bool
	WithOpenAPI     bool
	WithSkill       bool
	WithInteraction bool
	WithConfig      bool
	WithDB          bool
	ConfigMode      string
	RemoteProvider  string
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
	Project         *ir.Project
	ImportPath      string
	WithConfig      bool
	WithDB          bool
	WithGRPC        bool
	WithOpenAPI     bool
	WithSkill       bool
	WithInteraction bool
}

type goModTemplateData struct {
	ImportPath   string
	GoKitVersion string
	WithConfig   bool
	WithOpenAPI  bool
	RootRelPath  string
}
