package kit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Module is a named assembly unit attached to a Host.
//
// A module is how a capability — a database handle, a cache, a resource
// surface — brings its own wiring instead of making main assemble it. It is
// not a dynamic plugin: the set is fixed when the Host is built, and every
// module is a Go value the application compiled in.
//
// A module may also implement ModuleRegistrar, ModuleInitializer,
// ModuleShutdowner and ModuleDependencies. An empty name, a duplicated name, a
// requirement on a module that was not attached, and a dependency cycle are all
// rejected by NewHost, before anything runs.
type Module interface {
	Name() string
}

// ModuleDependencies is implemented by modules that must initialize after
// other modules.
//
// The names are module names, not registry keys. A missing value is not a
// dependency error — ordering is about who runs first, and a module that needs
// a value looks it up during Init, where it can report what it wanted and why.
type ModuleDependencies interface {
	Requires() []string
}

// ModuleRegistrar is implemented by modules that publish values. Every
// Register call on the Host runs before any Init call, so a module can depend
// on a value published by a module that initializes after it.
//
// Register must only publish: it takes part in startup, so it must not block
// on I/O or start anything. Work that can fail late belongs in Init.
type ModuleRegistrar interface {
	Register(*Registry) error
}

// ModuleInitializer is implemented by modules that start. Init runs after
// every Register has completed and before any lifecycle component starts, so a
// value a module publishes is available to every handler the process installs.
//
// An error stops the Host: a module that could not initialize leaves the
// process in a state its handlers were not written for.
type ModuleInitializer interface {
	Init(context.Context, *Registry) error
}

// ModuleShutdowner is implemented by modules that acquire something which has
// to be released — a connection pool, a lease, a file handle, a watcher.
//
// Shutdown runs after every lifecycle component has stopped, in reverse
// initialization order. That ordering is the reason this interface exists at
// all: handlers read the values modules published, so releasing a pool before
// the server stops would fail the requests the drain was still finishing.
//
// Teardown mirrors initialization: a module with no Init phase acquired nothing,
// one whose Init failed never claimed to hold anything, and one that did
// initialize is released at most once however the process got here — a failed
// Init, a failed Start, or a cancelled Run.
//
// An error is joined into the Host's result and does not stop the modules behind
// it: whatever else acquired something still has to let go.
type ModuleShutdowner interface {
	Shutdown(context.Context, *Registry) error
}

// Registry holds the values modules publish during startup.
//
// A Registry belongs to one Host. There is no package-level instance and no
// way to reach another Host's registry, which is what separates this from the
// global registries ARCHITECTURE.md rules out: the lookup table dies with the
// Host, and two Hosts in one process cannot see each other's values.
//
// Lookup is safe from any goroutine, so a handler may read a value during a
// request. Provide is meant for startup; calling it after Run started is
// allowed but is a design smell, because a handler may already have cached the
// absence of the value.
type Registry struct {
	mu     sync.RWMutex
	values map[string]any
}

// NewRegistry creates an empty registry. A Host creates its own; this
// constructor exists so a module can be tested without a Host.
func NewRegistry() *Registry {
	return &Registry{values: make(map[string]any)}
}

// Provide publishes a value under a name. Providing the same name twice is an
// error rather than an overwrite: two modules publishing one key is a
// configuration mistake, and silently letting the later one win makes the
// outcome depend on startup order.
//
// A nil value — including a typed nil pointer — is rejected. A lookup that
// succeeds and hands back a nil is worse than one that fails: the caller only
// finds out when it dereferences the value, which is inside a request.
func (r *Registry) Provide(name string, value any) error {
	if r == nil {
		return fmt.Errorf("kit: provide %q on a nil registry", name)
	}
	if name == "" {
		return fmt.Errorf("kit: registry key cannot be empty")
	}
	if isNilValue(value) {
		return fmt.Errorf("kit: registry value %q is nil", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]any)
	}
	if _, exists := r.values[name]; exists {
		return fmt.Errorf("kit: registry value %q is already provided", name)
	}
	r.values[name] = value
	return nil
}

// Lookup returns the value published under name.
func (r *Registry) Lookup(name string) (any, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.values[name]
	return value, ok
}

// Names lists the published keys in sorted order. It is meant for diagnostics
// — an Init that cannot find the value it needs can say what was there.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.values))
	for name := range r.values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns the value published under name, typed as T.
//
// The two failures it can report are different problems and are named
// separately: a missing key means the module that should have published it did
// not run, and a type mismatch means the lookup and the publisher disagree
// about the contract between them.
func Get[T any](r *Registry, name string) (T, error) {
	var zero T
	value, ok := r.Lookup(name)
	if !ok {
		return zero, fmt.Errorf("kit: registry has no value %q (published: %s)", name, describeNames(r.Names()))
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("kit: registry value %q is %s, not %s", name, typeNameOf(value), typeName[T]())
	}
	return typed, nil
}

// WithModules attaches modules to the Host.
//
// Modules initialize in a deterministic topological order derived from
// Requires. Two Hosts built from the same modules in a different declaration
// order produce the same startup sequence, which is what makes a startup log
// comparable between deployments.
//
//	host, err := kit.NewHost(
//	    kit.WithModules(pgModule, cacheModule, resourceModule),
//	    kit.WithLifecycle(api),
//	)
//	if err != nil {
//	    return err // a nil, unnamed, duplicated, unknown or cyclic module
//	}
//	return host.Run(ctx) // Init runs before the HTTP component starts
//
// Stable: kit.module-order — attached modules initialize in a deterministic topological order derived from Requires, whatever order they were attached in, and an invalid graph is rejected when the Host is built.
// Covered by: TestModuleStartupOrderIsDeterministic, TestModuleRequiresOrdersStartup, TestModuleGraphErrorsAreReportedAtConstruction
func WithModules(modules ...Module) HostOption {
	copied := append([]Module(nil), modules...)
	return func(h *Host) error {
		for i, module := range copied {
			if isNilModule(module) {
				return fmt.Errorf("module %d is nil", i)
			}
		}
		h.modules = append(h.modules, copied...)
		return nil
	}
}

// Registry returns the values the attached modules published.
//
// It is the other half of WithModules. A module can open a connection and
// publish it, but the code that registers routes runs in main, outside any
// module, and it needs the handle: without this accessor a published value is
// only reachable by another module, which makes publication a private
// conversation rather than a contract.
//
// Read it after Init. Before that the registry exists but is empty — a lookup
// there reports a missing value, which is the answer to "I did not initialize
// yet" that a caller can act on, rather than a nil dereference.
//
// Stable: kit.registry-is-readable — the values modules publish are readable by the assembling application through the Host's registry, not only by other modules.
// Covered by: TestPublishedValueReachesTheAssemblingApplication
func (h *Host) Registry() *Registry { return h.registry }

// Init runs the two phases modules are attached for: every Register, then
// every Init, both in dependency order.
//
// Run calls Init itself, so an application that uses Run never calls this.
// It is exported for a caller that assembles startup by hand and needs module
// initialization to happen at a point of its own choosing — and for a test
// that wants to assert a module's behavior without starting a listener.
//
// Init is idempotent: a second call after a successful one does nothing. A
// failed Init is terminal — the modules that did initialize are still holding
// whatever they acquired, so retrying would publish duplicate values and run
// half the startup twice. Every later call returns the first failure.
func (h *Host) Init(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil init context")
	}
	h.initMu.Lock()
	defer h.initMu.Unlock()

	h.mu.Lock()
	done, previousErr, started, stopped := h.initDone, h.initErr, h.started, h.stopped
	h.mu.Unlock()
	if done {
		return previousErr
	}
	if stopped {
		return fmt.Errorf("kit: host is already shut down; modules cannot initialize")
	}
	if started {
		return fmt.Errorf("kit: host is already started; modules must initialize before the Host starts")
	}

	// Module code runs without the diagnostic lock held: a module that reports
	// progress through the Host would otherwise deadlock against its own startup.
	initialized, initErr := h.initializeModules(ctx)

	h.mu.Lock()
	h.initDone = true
	h.initErr = initErr
	h.initialized = initialized
	h.mu.Unlock()
	return initErr
}

// initializeModules is the two-phase startup. It is called with initMu held and
// runs module code without holding the diagnostic lock, so a module that calls
// back into Host state cannot deadlock.
//
// It returns the modules that finished Init, in initialization order, because
// that is the set teardown has to walk backwards. A module whose Register ran and
// whose Init failed is not in the list: it never claimed to hold anything.
func (h *Host) initializeModules(ctx context.Context) ([]int, error) {
	for _, index := range h.moduleOrder {
		registrar, ok := h.modules[index].(ModuleRegistrar)
		if !ok {
			continue
		}
		if err := registrar.Register(h.registry); err != nil {
			return nil, fmt.Errorf("kit: register module %s: %w", h.modules[index].Name(), err)
		}
	}
	initialized := make([]int, 0, len(h.moduleOrder))
	for _, index := range h.moduleOrder {
		initializer, ok := h.modules[index].(ModuleInitializer)
		if !ok {
			continue
		}
		if err := initializer.Init(ctx, h.registry); err != nil {
			return initialized, fmt.Errorf("kit: initialize module %s: %w", h.modules[index].Name(), err)
		}
		initialized = append(initialized, index)
	}
	return initialized, nil
}

// shutdownModules releases what the modules acquired, in reverse initialization
// order. It runs once: the paths that reach it — a cancelled Run, a failed
// startup, an explicit Shutdown — must not hand a pool back twice.
func (h *Host) shutdownModules(ctx context.Context) error {
	h.mu.Lock()
	if h.modulesShutdown || len(h.initialized) == 0 {
		h.modulesShutdown = true
		h.mu.Unlock()
		return nil
	}
	h.modulesShutdown = true
	h.stopped = true
	order := append([]int(nil), h.initialized...)
	h.mu.Unlock()

	var result error
	for i := len(order) - 1; i >= 0; i-- {
		index := order[i]
		shutdowner, ok := h.modules[index].(ModuleShutdowner)
		if !ok {
			continue
		}
		if err := shutdowner.Shutdown(ctx, h.registry); err != nil {
			result = errors.Join(result, fmt.Errorf("kit: shut down module %s: %w", h.modules[index].Name(), err))
		}
	}
	return result
}

// initModules validates the module graph and prepares the registry. Errors
// here are reported by NewHost, because a graph that cannot be ordered is a
// build-time mistake that must not reach a listener.
//
// The registry is created whether or not modules are attached: Register,
// Init and Shutdown all receive one, and Registry() answers the caller that
// assembles the process. A nil registry would make each of those a special case.
func (h *Host) initModules() error {
	h.registry = NewRegistry()
	if len(h.modules) == 0 {
		return nil
	}
	order, err := orderModules(h.modules)
	if err != nil {
		return err
	}
	h.moduleOrder = order
	return nil
}

// orderModules returns the module indices in initialization order.
//
// The order is a topological sort with the name as the tie-break, so it depends
// on the set of modules and not on the order they were attached in. A ready
// list sorted by name is used instead of a queue seeded by attachment order for
// exactly that reason: a Host must not start differently because somebody
// reordered arguments.
func orderModules(modules []Module) ([]int, error) {
	names := make([]string, len(modules))
	index := make(map[string]int, len(modules))
	for i, module := range modules {
		name := module.Name()
		if name == "" {
			return nil, fmt.Errorf("kit: module %d has an empty name", i)
		}
		if _, duplicate := index[name]; duplicate {
			return nil, fmt.Errorf("kit: module %s is attached twice", name)
		}
		names[i] = name
		index[name] = i
	}

	indegree := make([]int, len(modules))
	dependents := make([][]int, len(modules))
	for i, module := range modules {
		dependent, ok := module.(ModuleDependencies)
		if !ok {
			continue
		}
		seen := make(map[string]bool)
		for _, required := range dependent.Requires() {
			if required == names[i] {
				return nil, fmt.Errorf("kit: module dependency cycle: %s -> %s", names[i], names[i])
			}
			target, ok := index[required]
			if !ok {
				return nil, fmt.Errorf("kit: module %s requires %s, which is not attached", names[i], required)
			}
			if seen[required] {
				continue
			}
			seen[required] = true
			dependents[target] = append(dependents[target], i)
			indegree[i]++
		}
	}

	ready := make([]int, 0, len(modules))
	for i, degree := range indegree {
		if degree == 0 {
			ready = append(ready, i)
		}
	}
	order := make([]int, 0, len(modules))
	for len(ready) > 0 {
		sort.Slice(ready, func(a, b int) bool { return names[ready[a]] < names[ready[b]] })
		next := ready[0]
		ready = ready[1:]
		order = append(order, next)
		for _, dependent := range dependents[next] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if len(order) != len(modules) {
		remaining := make(map[int]bool, len(modules))
		for i := range modules {
			remaining[i] = indegree[i] > 0
		}
		return nil, fmt.Errorf("kit: module dependency cycle: %s",
			strings.Join(cyclePath(modules, names, index, remaining), " -> "))
	}
	return order, nil
}

// cyclePath names one cycle among the modules the topological sort could not
// place. Reporting "a cycle exists" without the members leaves the reader to
// reconstruct it from Requires implementations.
func cyclePath(modules []Module, names []string, index map[string]int, remaining map[int]bool) []string {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make([]int, len(modules))
	var stack []int
	var walk func(int) []string
	walk = func(current int) []string {
		state[current] = visiting
		stack = append(stack, current)
		if dependent, ok := modules[current].(ModuleDependencies); ok {
			for _, required := range dependent.Requires() {
				next, ok := index[required]
				if !ok || !remaining[next] {
					continue
				}
				switch state[next] {
				case visiting:
					for position, entry := range stack {
						if entry != next {
							continue
						}
						path := make([]string, 0, len(stack)-position+1)
						for _, member := range stack[position:] {
							path = append(path, names[member])
						}
						return append(path, names[next])
					}
				case unvisited:
					if path := walk(next); path != nil {
						return path
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[current] = visited
		return nil
	}
	for i := range modules {
		if !remaining[i] || state[i] != unvisited {
			continue
		}
		if path := walk(i); path != nil {
			return path
		}
	}
	return []string{"?"}
}

// describeNames renders a published-key list for an error message.
func describeNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// typeNameOf names the dynamic type of a published value.
func typeNameOf(value any) string {
	if value == nil {
		return "nil"
	}
	return reflect.TypeOf(value).String()
}

// typeName names T, including when T is an interface.
func typeName[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().String()
}

// isNilModule reports whether a module is absent, including the typed nil that
// an interface holding a nil pointer looks like.
func isNilModule(module Module) bool {
	if module == nil {
		return true
	}
	return isNilValue(module)
}

// isNilValue reports whether a value is nil, including a typed nil inside an
// interface.
func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
