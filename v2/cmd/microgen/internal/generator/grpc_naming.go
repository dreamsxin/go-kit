package generator

import (
	"fmt"
	"strings"
)

// grpcProtoPackage composes the proto package of a generated service: the
// route prefix's segments (lowercased, joined with dots) in front of the
// service's own package name. It is what the generated .proto declares and
// what a gRPC client addresses — the same prefix the HTTP routes carry,
// expressed in the dot-separated form the gRPC wire path requires.
//
// A prefix segment must be a lowercase letter followed by lowercase letters,
// digits, or underscores: proto package identifiers are stricter than URL
// paths, and a malformed segment is refused at generation time rather than
// silently producing a contract no protoc will compile.
func grpcProtoPackage(routePrefix, servicePackageName string) (string, error) {
	pkg := servicePackageName
	if prefix := grpcPrefixSegments(routePrefix); prefix != "" {
		pkg = prefix + "." + pkg
	}
	if err := validateProtoPackage(pkg); err != nil {
		return "", err
	}
	return pkg, nil
}

// grpcPrefixSegments converts the HTTP route prefix into dot-separated proto
// package segments (" Receive" of "/api/v1" yields "api.v1").
func grpcPrefixSegments(routePrefix string) string {
	segments := strings.Split(strings.Trim(strings.TrimSpace(routePrefix), "/"), "/")
	var kept []string
	for _, segment := range segments {
		if segment = strings.TrimSpace(strings.ToLower(segment)); segment != "" {
			kept = append(kept, segment)
		}
	}
	return strings.Join(kept, ".")
}

// validateRoutePrefix checks the segments a route prefix contributes: they
// become HTTP path segments and, in dot form, the proto package prefix, so
// both grammars must accept them. A lowercase letter first, then letters,
// digits, or underscores — the same rule the proto package applies.
func validateRoutePrefix(routePrefix string) error {
	for _, segment := range strings.Split(strings.Trim(strings.TrimSpace(routePrefix), "/"), "/") {
		if segment == "" {
			continue
		}
		for i, r := range segment {
			valid := r == '_' ||
				(r >= 'a' && r <= 'z') ||
				(i > 0 && r >= '0' && r <= '9')
			if !valid {
				return fmt.Errorf("prefix segment %q is not usable: want lowercase letter first, then letters, digits, or underscores (the prefix names both the HTTP path and the proto package)", segment)
			}
		}
	}
	return nil
}

// validateProtoPackage refuses a package name protoc would reject.
func validateProtoPackage(pkg string) error {
	for _, segment := range strings.Split(pkg, ".") {
		if segment == "" {
			return fmt.Errorf("grpc package %q has an empty segment", pkg)
		}
		for i, r := range segment {
			valid := r == '_' ||
				(r >= 'a' && r <= 'z') ||
				(i > 0 && r >= '0' && r <= '9')
			if !valid {
				return fmt.Errorf("grpc package %q: segment %q is not a valid proto identifier (want lowercase letter first, then letters, digits, or underscores)", pkg, segment)
			}
		}
	}
	return nil
}
