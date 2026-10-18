package ir

import "strings"

// UnaryMethods returns the service's unary methods, keeping their declaration
// order. A nil method is skipped; a method with no Kind is unary, because that
// is what the parsers produce when the IDL did not say otherwise. It is a
// projection of the IR rather than a generator decision, which is why both the
// generator and the contract package read it from here.
func UnaryMethods(service *Service) []*Method {
	if service == nil {
		return nil
	}
	out := make([]*Method, 0, len(service.Methods))
	for _, method := range service.Methods {
		if method == nil {
			continue
		}
		if method.Kind == "" || method.Kind == MethodKindUnary {
			out = append(out, method)
		}
	}
	return out
}

// HasModels reports whether any message in the contract declares database
// model metadata. Model and repository output is built per such message; a
// contract with none has nothing to generate there, so the runtime that imports
// those packages would not compile.
func HasModels(project *Project) bool {
	if project == nil {
		return false
	}
	for _, message := range project.Messages {
		if message != nil && message.HasGormTags {
			return true
		}
	}
	return false
}

// RoutePrefix joins a configured prefix with a service name: the prefix gains
// a leading and a trailing slash if it lacks them, and the service name is
// lowercased, so "api/v1" + "UserService" is "/api/v1/userservice". An empty
// prefix stays empty, which is what keeps an unprefixed project unprefixed.
func RoutePrefix(basePrefix, serviceName string) string {
	if basePrefix == "" {
		return ""
	}
	prefix := basePrefix
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix + strings.ToLower(serviceName)
}
