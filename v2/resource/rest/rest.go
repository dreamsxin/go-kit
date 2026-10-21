// Package rest exposes a resource.Collection over HTTP.
//
// It is a binding, not a second stack. A route is returned as a value — the
// pattern, the endpoint, and the decoder that fills its request — so an
// application mounts it on whatever it assembles with:
//
//	routes, err := rest.Routes(products)
//	if err != nil {
//		return err
//	}
//	for _, route := range routes {
//		kit.HandleEndpoint(http, route.Pattern, route.Endpoint, route.Decode)
//	}
//
// A plain *http.ServeMux works too, through Route.Handler:
//
//	for _, route := range routes {
//		mux.Handle(route.Pattern, route.Handler())
//	}
//
// The two differ in what surrounds the endpoint, not in what the endpoint does:
// the kit path keeps the component's endpoint middleware, recorders, and JSON
// server options, while a bare handler has only what net/http gives it. The
// response body, status, and error code are the same either way, because both
// use the transport's JSON encoder and error encoder.
//
// What the binding reads from a request is deliberately small and explicit:
// `filter`, `sort`, `fields`, `append`, `page`, `size`, and — on the aggregate
// route — `function`, `field` and `group_by`. A parameter that does not apply
// to a route is refused rather than ignored, because a filter that was silently
// dropped answers a broader question than the one asked.
package rest

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/resource"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// Route is one HTTP route as a value: a pattern to register, the endpoint that
// answers it, and the decoder that turns a request into the endpoint's input.
//
// It is a value rather than a registration call so the same table mounts on a
// kit component and on a plain mux without the table knowing which. Nothing
// here is registered by being created.
type Route struct {
	// Pattern is a method-qualified pattern such as "GET /products/{id}". It
	// is valid for net/http's ServeMux and for a kit component, which uses
	// one.
	Pattern string
	// Endpoint is the business call the route makes.
	Endpoint endpoint.Endpoint
	// Decode reads the request into the value Endpoint receives.
	Decode httpserver.DecodeRequestFunc
}

// Handler returns a standalone http.Handler for the route, for an application
// that registers on a plain mux rather than through an assembly.
//
// It carries none of a component's endpoint middleware or recorders — a bare
// handler has no component to ask — but it encodes responses and errors through
// the same transport functions a component uses, so the wire answer does not
// depend on how the route was mounted.
func (r Route) Handler() http.Handler {
	if r.Endpoint == nil || r.Decode == nil {
		panic("rest: route has no endpoint or decoder; a zero Route is not registerable")
	}
	return httpserver.NewServer(r.Endpoint, r.Decode, httpserver.EncodeJSONResponse,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder))
}

// Action names one operation a route performs, and is what a policy layer is
// asked about.
type Action string

// The operations a collection exposes.
const (
	// ActionList reads a filtered, sorted, projected window of records.
	ActionList Action = "list"
	// ActionGet reads one record.
	ActionGet Action = "get"
	// ActionCreate stores a new record.
	ActionCreate Action = "create"
	// ActionUpdate changes the fields a patch names.
	ActionUpdate Action = "update"
	// ActionDestroy removes one record.
	ActionDestroy Action = "destroy"
	// ActionAggregate computes one figure over the records a filter selects.
	ActionAggregate Action = "aggregate"
)

// Layer returns the parameters one authority adds to an operation.
//
// It runs after the client's parameters are read and is merged with them by
// resource.MergeParams, so a layer can only narrow: another filter to satisfy,
// fewer columns, a smaller page. That ordering is the reason this is a function
// and not a pre-merged value — the layer needs to know which operation it is
// answering before it can say what it allows.
type Layer func(ctx context.Context, action Action) (resource.Params, error)

// Option configures the routes a collection exposes.
type Option func(*settings)

type settings struct {
	prefix       string
	maxBodyBytes int64
	defaults     resource.Params
	layers       []Layer
}

// WithPrefix mounts every route under a path prefix.
//
// The prefix is normalised to start with a slash and to carry none at the end,
// so "/api/v1", "api/v1" and "/api/v1/" mean the same mount. Braces are
// refused, because a prefix is a fixed path and a wildcard in it would let one
// collection answer another's requests.
func WithPrefix(prefix string) Option {
	return func(s *settings) { s.prefix = prefix }
}

// WithMaxBodyBytes bounds the JSON body of the create and update routes.
//
// A value of zero or less means the transport default; it does not mean "no
// limit", which is a decision an application makes once for its server rather
// than per collection.
func WithMaxBodyBytes(maxBodyBytes int64) Option {
	return func(s *settings) { s.maxBodyBytes = maxBodyBytes }
}

// WithDefaults sets the resource's own layer: the parameters that apply to every
// request before the client's are merged in.
//
// It is where a default sort, a default page size smaller than the transport's,
// or a resource-wide filter belongs. The client's parameters are merged after
// it, so a default can be narrowed but not widened.
func WithDefaults(params resource.Params) Option {
	return func(s *settings) { s.defaults = params }
}

// WithLayer adds an authority whose parameters narrow every request.
//
// This is the seam a policy layer plugs into: the layer sees the action and the
// context, returns a resource.Params, and the merge guarantees it cannot be
// widened by anything the client sent. Several layers are applied in the order
// given.
func WithLayer(layers ...Layer) Option {
	return func(s *settings) {
		for _, layer := range layers {
			if layer != nil {
				s.layers = append(s.layers, layer)
			}
		}
	}
}

// Routes returns the routes a collection exposes.
//
// Stable: rest.route-table — a collection answers at {prefix}/{name} for list and create, {prefix}/{name}:aggregate for aggregates, and {prefix}/{name}/{id} for get, update and destroy.
// Covered by: TestRoutesAreTheDocumentedTable, TestRoutesUnderAPrefix
func Routes(collection resource.Collection, options ...Option) ([]Route, error) {
	if err := collection.Validate(); err != nil {
		return nil, err
	}
	if len(collection.Primary) != 1 {
		return nil, apperror.InvalidArgument("resource.key_not_addressable",
			fmt.Sprintf("collection %q has a composite primary key (%s); an HTTP route addresses one record with one path segment",
				collection.Name, strings.Join(collection.Primary, ", ")))
	}
	binding, err := newBinding(collection, options...)
	if err != nil {
		return nil, err
	}
	base := binding.settings.prefix + "/" + collection.Name
	return []Route{
		binding.route(http.MethodGet, base, ActionList, binding.decodeList),
		binding.route(http.MethodPost, base, ActionCreate, binding.decodeCreate),
		binding.route(http.MethodGet, base+":aggregate", ActionAggregate, binding.decodeAggregate),
		binding.route(http.MethodGet, base+"/{id}", ActionGet, binding.decodeGet),
		binding.route(http.MethodPatch, base+"/{id}", ActionUpdate, binding.decodeUpdate),
		binding.route(http.MethodDelete, base+"/{id}", ActionDestroy, binding.decodeDestroy),
	}, nil
}

// binding is one collection plus the settings its routes share.
type binding struct {
	collection resource.Collection
	settings   settings
	defaults   resource.Params
}

func newBinding(collection resource.Collection, options ...Option) (*binding, error) {
	var settings settings
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	prefix := strings.TrimSpace(settings.prefix)
	if prefix != "" {
		if strings.ContainsAny(prefix, "{}") {
			return nil, apperror.InvalidArgument("rest.prefix_invalid",
				fmt.Sprintf("prefix %q contains a wildcard; a prefix is a fixed path", settings.prefix))
		}
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		prefix = strings.TrimSuffix(prefix, "/")
	}
	settings.prefix = prefix
	if settings.maxBodyBytes <= 0 {
		settings.maxBodyBytes = httpserver.DefaultMaxJSONBodyBytes
	}

	// The writable set is derived from the schema, not left open. A mask with
	// no whitelist allows every field, and a repository that trusted it would
	// accept a column the schema marked unwritable — the mask is the layer that
	// makes the refusal independent of which adapter is behind it.
	defaults := settings.defaults
	defaults.Writable = resource.MergeWriteMasks(defaults.Writable,
		resource.WriteMask{Allow: writableFields(collection.Schema)})
	if err := defaults.Validate(collection.Schema); err != nil {
		return nil, err
	}
	return &binding{collection: collection, settings: settings, defaults: defaults}, nil
}

// route assembles one route: the endpoint wrapped so the handler sees a typed
// request, and the decoder that produced it.
func (b *binding) route(method, pattern string, action Action, decode httpserver.DecodeRequestFunc) Route {
	ep := func(ctx context.Context, raw any) (any, error) {
		request, ok := raw.(*request)
		if !ok || request == nil {
			return nil, apperror.Internal("rest.request_missing",
				"the route decoded no request")
		}
		return b.handle(ctx, action, request)
	}
	return Route{Pattern: method + " " + pattern, Endpoint: ep, Decode: decode}
}

// handle dispatches one decoded request to its operation.
func (b *binding) handle(ctx context.Context, action Action, request *request) (any, error) {
	switch action {
	case ActionList:
		return b.list(ctx, request)
	case ActionGet:
		return b.get(ctx, request)
	case ActionCreate:
		return b.create(ctx, request)
	case ActionUpdate:
		return b.update(ctx, request)
	case ActionDestroy:
		return b.destroy(ctx, request)
	case ActionAggregate:
		return b.aggregate(ctx, request)
	default:
		return nil, apperror.Internal("rest.action_unknown",
			fmt.Sprintf("route action %q is not implemented", string(action)))
	}
}

// writableFields lists the fields the schema allows a write to name.
//
// It returns a non-nil slice even when there are none: nil would mean "no
// opinion", and a collection with no writable field must refuse every write
// rather than allow every one.
func writableFields(schema resource.Schema) []string {
	fields := make([]string, 0, schema.Len())
	for _, field := range schema.Fields() {
		if field.Writable {
			fields = append(fields, field.Name)
		}
	}
	return fields
}
