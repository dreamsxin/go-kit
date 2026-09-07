package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Protocol extensions.
//
// 2026-07-28 gave extensions formal standing: an extension has a reverse-DNS id
// ("vendor.example/name"), a version that moves independently of the protocol
// revision, and a configuration object of its own, and both sides declare the
// ones they support in their capabilities. Two rules follow from that, and this
// package holds both: an extension is off until someone enables it, and a peer
// that does not support one falls back to core behaviour rather than failing.
//
// Which extensions a server offers is an application decision — the framework
// implements none and invents none. A deployment describes one with Extension,
// registers it with StreamableHandler.RegisterExtension, and the transport
// declares it, routes its methods, and keeps it out of the way of everything
// else.

const (
	// reservedExtensionPrefix belongs to the specification. An application that
	// declared an id under it would be claiming to implement an official
	// extension, which a client is entitled to believe.
	reservedExtensionPrefix = "io.modelcontextprotocol/"
)

// ExtensionMethod answers one request of an extension's own method. The context
// is the request's, so an implementation reads identity, client capabilities and
// request `_meta` from it exactly as a tool does.
//
// The returned value is the JSON-RPC result. An error is reported with the code
// its interaction sentinel maps to, or -32603 when it names none: an extension
// that fails is a server fault, not a malformed request.
type ExtensionMethod func(ctx context.Context, params json.RawMessage) (any, error)

// Extension is one namespaced protocol extension a deployment implements.
type Extension struct {
	// Name is the extension id: a reverse-DNS vendor prefix, a slash, and the
	// extension's own name — "vendor.example/audit". The
	// "io.modelcontextprotocol/" prefix is the specification's and is refused
	// here.
	Name string

	// Version is the extension's own version, which moves independently of the
	// protocol revision. It is required: an extension without one cannot be
	// depended on by a client that must decide whether it understands this one.
	Version string

	// Config is the extension's configuration object, reported verbatim beside
	// its version so a client can read what this deployment enabled. It is
	// optional, and the extension defines its schema.
	Config map[string]any

	// Methods are the JSON-RPC methods the extension adds, each keyed by its
	// full name, which must begin with the extension id and a slash. A method
	// answers requests only: a notification carries no id to answer and is
	// acknowledged by the transport without reaching an extension.
	Methods map[string]ExtensionMethod
}

// RegisterExtension enables one extension on this handler. Call it before the
// handler serves traffic; registration is not synchronised against in-flight
// requests.
//
// Stable: mcp.extension-namespace — an extension id must be reverse-DNS namespaced, its methods must carry that id as their prefix, and the specification's own namespace is refused.
// Covered by: TestRegisterExtensionRefusesAnUnusableDeclaration
//
// Stable: mcp.extension-declaration — capabilities report each registered extension with its version and configuration, and report none when nothing was registered.
// Covered by: TestExtensionsAreDeclaredOnlyWhenRegistered
func (h *StreamableHandler) RegisterExtension(ext Extension) error {
	name := strings.TrimSpace(ext.Name)
	switch {
	case name == "":
		return errors.New("mcp: extension name is required")
	case strings.HasPrefix(name, reservedExtensionPrefix):
		return fmt.Errorf("mcp: extension %q claims the specification's %s namespace", name, reservedExtensionPrefix)
	case strings.ContainsAny(name, " \t"):
		return fmt.Errorf("mcp: extension %q contains whitespace", name)
	}
	vendor, own, ok := strings.Cut(name, "/")
	if !ok || own == "" || !strings.Contains(vendor, ".") {
		return fmt.Errorf("mcp: extension %q is not a reverse-DNS id like \"vendor.example/name\"", name)
	}
	if strings.TrimSpace(ext.Version) == "" {
		return fmt.Errorf("mcp: extension %q declares no version", name)
	}
	if _, taken := h.core.extensionsByName[name]; taken {
		return fmt.Errorf("mcp: extension %q is already registered", name)
	}

	for method := range ext.Methods {
		if !strings.HasPrefix(method, name+"/") {
			return fmt.Errorf("mcp: extension %q method %q does not carry its extension's namespace", name, method)
		}
		if ext.Methods[method] == nil {
			return fmt.Errorf("mcp: extension %q method %q has no implementation", name, method)
		}
		if coreMethodNames[method] {
			return fmt.Errorf("mcp: extension %q method %q is a core protocol method", name, method)
		}
		if _, taken := h.core.extensionMethods[method]; taken {
			return fmt.Errorf("mcp: method %q is already registered by another extension", method)
		}
	}

	if h.core.extensionsByName == nil {
		h.core.extensionsByName = map[string]Extension{}
	}
	if h.core.extensionMethods == nil {
		h.core.extensionMethods = map[string]ExtensionMethod{}
	}
	ext.Name = name
	h.core.extensionsByName[name] = ext
	for method, fn := range ext.Methods {
		h.core.extensionMethods[method] = fn
	}
	return nil
}

// coreMethodNames are the methods the protocol itself defines here. An extension
// may not take one over: a client sending "tools/call" must reach the tool
// registry, whatever a deployment registered.
var coreMethodNames = map[string]bool{
	"initialize":                true,
	"notifications/initialized": true,
	"ping":                      true,
	"server/discover":           true,
	"tools/list":                true,
	"tools/call":                true,
	"resources/list":            true,
	"resources/read":            true,
	"resources/templates/list":  true,
	"prompts/list":              true,
	"prompts/get":               true,
	"completion/complete":       true,
	"logging/setLevel":          true,
}

// declaredExtensions renders the extensions map that goes into capabilities.
func (c *dispatchCore) declaredExtensions() map[string]any {
	if len(c.extensionsByName) == 0 {
		return nil
	}
	declared := make(map[string]any, len(c.extensionsByName))
	for name, ext := range c.extensionsByName {
		entry := map[string]any{"version": ext.Version}
		if len(ext.Config) > 0 {
			entry["config"] = ext.Config
		}
		if len(ext.Methods) > 0 {
			entry["methods"] = sortedMethodNames(ext.Methods)
		}
		declared[name] = entry
	}
	return declared
}

func sortedMethodNames(methods map[string]ExtensionMethod) []string {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	// A capability document that reorders itself between calls is a document a
	// client cannot cache under the ttlMs this revision asks it to honour.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// dispatchExtension answers a method an extension registered, and reports
// whether one claimed it.
//
// Stable: mcp.extension-methods — a registered extension method is dispatched on both revisions, and an unregistered one stays -32601 so a client sees a server without the extension.
// Covered by: TestExtensionMethodIsDispatchedOnBothRevisions, TestUnregisteredExtensionMethodIsNotFound
func (c *dispatchCore) dispatchExtension(ctx context.Context, req request) (any, *rpcError, bool) {
	method, ok := c.extensionMethods[req.Method]
	if !ok {
		return nil, nil, false
	}
	result, err := method(ctx, req.Params)
	if err != nil {
		code := RPCCodeForInteractionError(err)
		return nil, newError(code, messageForCode(code), err.Error()), true
	}
	return result, nil, true
}

// ─── client-side extensions ──────────────────────────────────────────────────

// ClientExtensionFromContext reports the configuration a stateless request
// declared for one extension, and whether the client declared it at all.
//
// It is what the specification's fallback rule needs: a tool or extension method
// that could use an extension checks for it, and behaves like core protocol when
// the caller has no code for it — or refuses plainly when the extension is a hard
// prerequisite.
//
// Stable: mcp.client-extension-declaration — a client's declared extensions are readable per request, so an implementation can fall back to core behaviour instead of assuming.
// Covered by: TestClientExtensionIsReadablePerRequest
func ClientExtensionFromContext(ctx context.Context, name string) (map[string]any, bool) {
	identity, ok := ctx.Value(statelessContextKey{}).(requestIdentity)
	if !ok {
		return nil, false
	}
	declared, ok := identity.capabilities["extensions"].(map[string]any)
	if !ok {
		return nil, false
	}
	entry, ok := declared[name]
	if !ok {
		return nil, false
	}
	config, _ := entry.(map[string]any)
	return config, true
}

// MetaFromContext reports the `_meta` block a stateless request carried, keyed as
// the client sent it.
//
// Extensions travel in `_meta`, and this server cannot know every one of them.
// An unrecognised key is therefore never an error and never dropped on the way
// in: it is here, for the tool or extension method that does understand it.
//
// Stable: mcp.unknown-meta-preserved — an unrecognised `_meta` key is neither refused nor discarded; it reaches the implementation through MetaFromContext.
// Covered by: TestUnknownMetaKeyIsCarriedNotRefused
func MetaFromContext(ctx context.Context) (map[string]any, bool) {
	identity, ok := ctx.Value(statelessContextKey{}).(requestIdentity)
	if !ok {
		return nil, false
	}
	return identity.meta, true
}
