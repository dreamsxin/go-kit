package ir

import "testing"

func TestRoutePrefix(t *testing.T) {
	tests := []struct {
		name       string
		basePrefix string
		service    string
		want       string
	}{
		{name: "empty", basePrefix: "", service: "UserService", want: ""},
		{name: "no slash", basePrefix: "api/v1", service: "UserService", want: "/api/v1/userservice"},
		{name: "leading slash", basePrefix: "/v2", service: "Greeter", want: "/v2/greeter"},
		{name: "trailing slash", basePrefix: "/api/", service: "OrderService", want: "/api/orderservice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoutePrefix(tt.basePrefix, tt.service); got != tt.want {
				t.Fatalf("RoutePrefix(%q, %q) = %q, want %q", tt.basePrefix, tt.service, got, tt.want)
			}
		})
	}
}

// TestUnaryMethods pins the projection the generators rely on: declaration
// order, a nil method skipped, and a method with no Kind treated as unary —
// the last one is what a parser produces when the IDL did not say otherwise.
func TestUnaryMethods(t *testing.T) {
	var nilService *Service
	if got := UnaryMethods(nilService); got != nil {
		t.Fatalf("nil service: got %v, want nil", got)
	}

	unary := &Method{Name: "Get", Kind: MethodKindUnary}
	implicit := &Method{Name: "List"}
	serverStream := &Method{Name: "Watch", Kind: MethodKindServerStream}
	clientStream := &Method{Name: "Upload", Kind: MethodKindClientStream}
	bidi := &Method{Name: "Chat", Kind: MethodKindBidirectional}

	service := &Service{Methods: []*Method{unary, nil, implicit, serverStream, clientStream, bidi}}
	got := UnaryMethods(service)
	if len(got) != 2 || got[0] != unary || got[1] != implicit {
		t.Fatalf("UnaryMethods = %v, want [Get List] in declaration order", methodNames(got))
	}
}

func TestHasModels(t *testing.T) {
	if HasModels(nil) {
		t.Fatal("nil project: HasModels = true, want false")
	}
	if HasModels(&Project{}) {
		t.Fatal("empty project: HasModels = true, want false")
	}
	plain := &Project{Messages: []*Message{{Name: "Request"}, nil}}
	if HasModels(plain) {
		t.Fatal("messages without gorm tags: HasModels = true, want false")
	}
	withModel := &Project{Messages: []*Message{{Name: "Request"}, {Name: "User", HasGormTags: true}}}
	if !HasModels(withModel) {
		t.Fatal("a message with gorm tags: HasModels = false, want true")
	}
}

func methodNames(methods []*Method) []string {
	names := make([]string, 0, len(methods))
	for _, method := range methods {
		names = append(names, method.Name)
	}
	return names
}
