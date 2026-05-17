package generator

import "github.com/dreamsxin/go-kit/cmd/microgen/ir"

func unaryMethods(service *ir.Service) []*ir.Method {
	if service == nil {
		return nil
	}
	out := make([]*ir.Method, 0, len(service.Methods))
	for _, method := range service.Methods {
		if method.Kind == "" || method.Kind == ir.MethodKindUnary {
			out = append(out, method)
		}
	}
	return out
}

func serverStreamMethods(service *ir.Service) []*ir.Method {
	if service == nil {
		return nil
	}
	out := make([]*ir.Method, 0, len(service.Methods))
	for _, method := range service.Methods {
		if method.Kind == ir.MethodKindServerStream {
			out = append(out, method)
		}
	}
	return out
}

func unaryServiceRoutes(routes []SvcRoute) []SvcRoute {
	out := make([]SvcRoute, 0, len(routes))
	for _, route := range routes {
		methods := unaryMethods(route.Service)
		if len(methods) == 0 {
			continue
		}
		service := *route.Service
		service.Methods = methods
		route.Service = &service
		out = append(out, route)
	}
	return out
}
