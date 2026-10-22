package kit_test

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// startLog records the startup events of the modules under test in order.
type startLog struct {
	mu     sync.Mutex
	events []string
}

func (l *startLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *startLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// hookModule implements every optional module interface, so a test decides
// which phases it takes part in by leaving the hook nil.
type hookModule struct {
	name     string
	requires []string
	register func(*kit.Registry) error
	init     func(context.Context, *kit.Registry) error
}

func (m *hookModule) Name() string       { return m.name }
func (m *hookModule) Requires() []string { return m.requires }

func (m *hookModule) Register(registry *kit.Registry) error {
	if m.register == nil {
		return nil
	}
	return m.register(registry)
}

func (m *hookModule) Init(ctx context.Context, registry *kit.Registry) error {
	if m.init == nil {
		return nil
	}
	return m.init(ctx, registry)
}

// bareModule implements only Module.
type bareModule string

func (m bareModule) Name() string { return string(m) }

// moduleComponent is a lifecycle stub that records whether it was started.
type moduleComponent struct {
	mu     sync.Mutex
	starts int
	stops  int
}

func (c *moduleComponent) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starts++
	return nil
}

func (c *moduleComponent) Errors() <-chan error { return nil }

func (c *moduleComponent) Shutdown(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stops++
	return nil
}

func (c *moduleComponent) started() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.starts
}

// TestModuleStartupOrderIsDeterministic is the promise kit.module-order makes:
// the same set of modules produces the same startup sequence whatever order
// they were attached in.
//
// The expected sequence also pins the tie-break. Three modules have no
// requirements, and the order among them is by name — audit, db, metrics — not
// by declaration, which is what makes two deployments' startup logs comparable.
func TestModuleStartupOrderIsDeterministic(t *testing.T) {
	tracked := func(log *startLog) []kit.Module {
		track := func(name string, requires ...string) *hookModule {
			return &hookModule{
				name:     name,
				requires: requires,
				register: func(*kit.Registry) error {
					log.add("register:" + name)
					return nil
				},
				init: func(context.Context, *kit.Registry) error {
					log.add("init:" + name)
					return nil
				},
			}
		}
		return []kit.Module{
			track("api", "cache"),
			track("cache", "db"),
			track("db"),
			track("metrics"),
			track("audit"),
		}
	}

	want := []string{
		"register:audit", "register:db", "register:cache", "register:api", "register:metrics",
		"init:audit", "init:db", "init:cache", "init:api", "init:metrics",
	}

	for run := 0; run < 100; run++ {
		log := &startLog{}
		modules := tracked(log)
		rand.New(rand.NewSource(int64(run))).Shuffle(len(modules), func(a, b int) {
			modules[a], modules[b] = modules[b], modules[a]
		})

		host, err := kit.NewHost(kit.WithModules(modules...))
		if err != nil {
			t.Fatalf("run %d: NewHost: %v", run, err)
		}
		if err := host.Init(context.Background()); err != nil {
			t.Fatalf("run %d: Init: %v", run, err)
		}
		if got := log.snapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: startup sequence = %v, want %v", run, got, want)
		}
	}
}

// TestModuleRequiresOrdersStartup attaches the dependent first and requires the
// dependency, which must still initialize first.
func TestModuleRequiresOrdersStartup(t *testing.T) {
	log := &startLog{}
	dependency := &hookModule{name: "storage", init: func(context.Context, *kit.Registry) error {
		log.add("init:storage")
		return nil
	}}
	dependent := &hookModule{name: "app", requires: []string{"storage"}, init: func(context.Context, *kit.Registry) error {
		log.add("init:app")
		return nil
	}}

	host, err := kit.NewHost(kit.WithModules(dependent, dependency))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got, want := log.snapshot(), []string{"init:storage", "init:app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("initialization order = %v, want %v", got, want)
	}
}

// TestModuleGraphErrorsAreReportedAtConstruction checks that every invalid
// graph fails when the Host is built, not when the first request arrives.
func TestModuleGraphErrorsAreReportedAtConstruction(t *testing.T) {
	cases := []struct {
		name    string
		modules []kit.Module
		want    []string
	}{
		{
			name:    "nil module",
			modules: []kit.Module{nil},
			want:    []string{"module 0 is nil"},
		},
		{
			name:    "empty name",
			modules: []kit.Module{bareModule("")},
			want:    []string{"empty name"},
		},
		{
			name:    "duplicate name",
			modules: []kit.Module{bareModule("db"), bareModule("db")},
			want:    []string{"db is attached twice"},
		},
		{
			name:    "unknown requirement",
			modules: []kit.Module{&hookModule{name: "app", requires: []string{"ghost"}}},
			want:    []string{"app requires ghost", "not attached"},
		},
		{
			name:    "self cycle",
			modules: []kit.Module{&hookModule{name: "app", requires: []string{"app"}}},
			want:    []string{"cycle", "app -> app"},
		},
		{
			name: "cycle",
			modules: []kit.Module{
				&hookModule{name: "a", requires: []string{"c"}},
				&hookModule{name: "b", requires: []string{"a"}},
				&hookModule{name: "c", requires: []string{"b"}},
			},
			want: []string{"cycle", "a", "b", "c"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			host, err := kit.NewHost(kit.WithModules(testCase.modules...))
			if err == nil {
				t.Fatalf("NewHost accepted an invalid module graph: %#v", testCase.modules)
			}
			if host != nil {
				t.Fatalf("NewHost returned a Host alongside the error")
			}
			for _, fragment := range testCase.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error %q does not mention %q", err, fragment)
				}
			}
		})
	}
}

// TestModuleRegistersBeforeAnyInit is the property the two phases exist for: a
// module may use a value published by a module that initializes after it.
//
// Neither module declares a requirement, and "reader" sorts before "writer", so
// a merged single pass would initialize the reader first and fail.
func TestModuleRegistersBeforeAnyInit(t *testing.T) {
	log := &startLog{}
	reader := &hookModule{name: "reader", init: func(_ context.Context, registry *kit.Registry) error {
		log.add("init:reader")
		greeting, err := kit.Get[string](registry, "greeting")
		if err != nil {
			return err
		}
		if greeting != "hello" {
			return errors.New("unexpected greeting " + greeting)
		}
		return nil
	}}
	writer := &hookModule{name: "writer", register: func(registry *kit.Registry) error {
		log.add("register:writer")
		return registry.Provide("greeting", "hello")
	}}

	host, err := kit.NewHost(kit.WithModules(reader, writer))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	events := log.snapshot()
	if len(events) != 2 || events[0] != "register:writer" || events[1] != "init:reader" {
		t.Fatalf("startup events = %v, want every Register before any Init", events)
	}
}

// TestModuleInitFailurePreventsStartup checks that a module that cannot
// initialize stops the process before a listener accepts traffic.
func TestModuleInitFailurePreventsStartup(t *testing.T) {
	failure := errors.New("storage is unreachable")
	broken := &hookModule{name: "storage", init: func(context.Context, *kit.Registry) error {
		return failure
	}}
	component := &moduleComponent{}

	host, err := kit.NewHost(kit.WithModules(broken), kit.WithLifecycle(component))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	err = host.Run(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("Run error = %v, want it to wrap %v", err, failure)
	}
	if !strings.Contains(err.Error(), "initialize module storage") {
		t.Fatalf("Run error = %v, want it to name the module and the phase", err)
	}
	if starts := component.started(); starts != 0 {
		t.Fatalf("component started %d times after a failed module init, want 0", starts)
	}
}

// TestRunInitializesModules checks the path an application actually takes: Run
// initializes modules before starting the components.
func TestRunInitializesModules(t *testing.T) {
	initialized := make(chan struct{})
	module := &hookModule{name: "storage", init: func(context.Context, *kit.Registry) error {
		close(initialized)
		return nil
	}}
	component := &moduleComponent{}

	host, err := kit.NewHost(kit.WithModules(module), kit.WithLifecycle(component))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-initialized:
	default:
		t.Fatal("Run did not initialize the attached module")
	}
	if starts := component.started(); starts != 1 {
		t.Fatalf("component started %d times, want 1", starts)
	}
}

// TestHostStartRequiresModuleInitialization checks the diagnostic a caller gets
// for skipping Init when assembling startup by hand.
func TestHostStartRequiresModuleInitialization(t *testing.T) {
	host, err := kit.NewHost(kit.WithModules(bareModule("db")))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	err = host.Start()
	if err == nil || !strings.Contains(err.Error(), "call Init before Start") {
		t.Fatalf("Start error = %v, want it to ask for Init", err)
	}
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := host.Start(); err != nil {
		t.Fatalf("Start after Init: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestModuleInitIsIdempotent checks that a second Init neither re-registers nor
// re-initializes: a duplicate Provide would otherwise turn a correct startup
// into a failure.
func TestModuleInitIsIdempotent(t *testing.T) {
	var registers, inits int
	module := &hookModule{
		name:     "db",
		register: func(registry *kit.Registry) error { registers++; return registry.Provide("db", "conn") },
		init:     func(context.Context, *kit.Registry) error { inits++; return nil },
	}
	host, err := kit.NewHost(kit.WithModules(module))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := host.Init(context.Background()); err != nil {
			t.Fatalf("Init %d: %v", attempt, err)
		}
	}
	if registers != 1 || inits != 1 {
		t.Fatalf("register=%d init=%d, want one each", registers, inits)
	}
}

// TestModuleInitFailureIsTerminal checks that the first failure is the one every
// later call reports: retrying would publish duplicate values and run half of
// startup twice.
func TestModuleInitFailureIsTerminal(t *testing.T) {
	failure := errors.New("cannot connect")
	var attempts int
	module := &hookModule{name: "db", init: func(context.Context, *kit.Registry) error {
		attempts++
		return failure
	}}
	host, err := kit.NewHost(kit.WithModules(module))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	first := host.Init(context.Background())
	second := host.Init(context.Background())
	if !errors.Is(first, failure) || !errors.Is(second, failure) {
		t.Fatalf("Init errors = %v / %v, want both to wrap %v", first, second, failure)
	}
	if attempts != 1 {
		t.Fatalf("Init ran %d times, want 1", attempts)
	}
}

// TestInitRejectsNilContext checks the same nil-context contract Run and
// Shutdown already keep.
func TestInitRejectsNilContext(t *testing.T) {
	host := kit.MustNewHost(kit.WithModules(bareModule("db")))
	//lint:ignore SA1012 the nil context is the input under test
	if err := host.Init(nil); err == nil {
		t.Fatal("Init accepted a nil context")
	}
}

// TestRegistryRejectsAmbiguousValues checks the three mistakes a registry can
// be handed, each of which would otherwise make startup depend on order.
func TestRegistryRejectsAmbiguousValues(t *testing.T) {
	registry := kit.NewRegistry()
	if err := registry.Provide("db", "conn"); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	if err := registry.Provide("db", "other"); err == nil {
		t.Fatal("Provide overwrote an existing value")
	}
	if err := registry.Provide("", "value"); err == nil {
		t.Fatal("Provide accepted an empty key")
	}
	if err := registry.Provide("nil", nil); err == nil {
		t.Fatal("Provide accepted a nil value")
	}
	var absent *moduleComponent
	if err := registry.Provide("pointer", absent); err == nil {
		t.Fatal("Provide accepted a typed nil pointer")
	}
	if got, want := registry.Names(), []string{"db"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
}

// TestGetSeparatesMissingFromMistyped checks that the two lookup failures say
// which one happened: a module that forgot to publish and a module that
// published something else are different bugs.
func TestGetSeparatesMissingFromMistyped(t *testing.T) {
	registry := kit.NewRegistry()
	if err := registry.Provide("db", 42); err != nil {
		t.Fatalf("Provide: %v", err)
	}

	if _, err := kit.Get[string](registry, "cache"); err == nil {
		t.Fatal("Get found a value that was never provided")
	} else if !strings.Contains(err.Error(), `no value "cache"`) || !strings.Contains(err.Error(), "db") {
		t.Fatalf("missing-value error = %q, want the key and the keys that are published", err)
	}

	if _, err := kit.Get[string](registry, "db"); err == nil {
		t.Fatal("Get accepted a value of the wrong type")
	} else if !strings.Contains(err.Error(), "is int, not string") {
		t.Fatalf("type-mismatch error = %q, want both types", err)
	}

	value, err := kit.Get[int](registry, "db")
	if err != nil || value != 42 {
		t.Fatalf("Get[int] = %d, %v; want 42, nil", value, err)
	}
}

// closingModule adds the teardown phase to hookModule. It is a separate type so a
// module that holds nothing is not silently a Shutdowner: a test that means to
// assert the phase is missing would otherwise pass by doing nothing.
type closingModule struct {
	*hookModule
	shutdown func(context.Context, *kit.Registry) error
}

func (m *closingModule) Shutdown(ctx context.Context, registry *kit.Registry) error {
	if m.shutdown == nil {
		return nil
	}
	return m.shutdown(ctx, registry)
}

// recordingComponent writes its stop into the same log the modules use, which is
// what makes the ordering between the two phases visible in one sequence.
type recordingComponent struct{ log *startLog }

func (c *recordingComponent) Start() error         { return nil }
func (c *recordingComponent) Errors() <-chan error { return nil }

func (c *recordingComponent) Shutdown(context.Context) error {
	c.log.add("stop:component")
	return nil
}

// TestPublishedValueReachesTheAssemblingApplication is the promise
// kit.registry-is-readable makes: a module can open something and publish it, and
// the code in main that registers routes — which runs outside any module — can
// read it.
func TestPublishedValueReachesTheAssemblingApplication(t *testing.T) {
	host, err := kit.NewHost(kit.WithModules(&hookModule{
		name: "database",
		init: func(_ context.Context, registry *kit.Registry) error {
			return registry.Provide("database", "pool")
		},
	}))
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	handle, err := kit.Get[string](host.Registry(), "database")
	if err != nil {
		t.Fatalf("Get from the assembling application: %v", err)
	}
	if handle != "pool" {
		t.Fatalf("handle = %q, want %q", handle, "pool")
	}
}

// TestReadingBeforeInitReportsAMissingValue pins what the registry says to a
// caller that read it too early. An empty registry answers "not provided", which
// an application can act on; a nil registry would be a panic inside a handler.
func TestReadingBeforeInitReportsAMissingValue(t *testing.T) {
	host := kit.MustNewHost(kit.WithModules(&hookModule{name: "database"}))
	if _, err := kit.Get[string](host.Registry(), "database"); err == nil {
		t.Fatal("Get found a value before Init ran")
	}
}

// TestModuleShutdownRunsAfterComponentsInReverseOrder pins the ordering that
// makes the phase safe: handlers read what modules published, so a component's
// Shutdown has to be done before a module hands its resource back, and modules
// release in the opposite order to the one they acquired in.
func TestModuleShutdownRunsAfterComponentsInReverseOrder(t *testing.T) {
	log := &startLog{}
	module := func(name string, requires ...string) kit.Module {
		return &closingModule{
			hookModule: &hookModule{
				name:     name,
				requires: requires,
				init: func(context.Context, *kit.Registry) error {
					log.add("init:" + name)
					return nil
				},
			},
			shutdown: func(context.Context, *kit.Registry) error {
				log.add("close:" + name)
				return nil
			},
		}
	}
	component := &recordingComponent{log: log}

	host, err := kit.NewHost(
		kit.WithLifecycle(component),
		kit.WithModules(module("alpha"), module("beta", "alpha")),
	)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	want := []string{"init:alpha", "init:beta", "stop:component", "close:beta", "close:alpha"}
	if got := log.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// TestModuleShutdownErrorsAreJoinedAndDoNotStopTheRest checks that a module which
// fails to let go is reported without stranding the modules behind it.
func TestModuleShutdownErrorsAreJoinedAndDoNotStopTheRest(t *testing.T) {
	failure := errors.New("pool is stuck")
	var closed []string
	module := func(name string, err error) kit.Module {
		return &closingModule{
			hookModule: &hookModule{name: name, init: func(context.Context, *kit.Registry) error { return nil }},
			shutdown: func(context.Context, *kit.Registry) error {
				closed = append(closed, name)
				return err
			},
		}
	}
	host := kit.MustNewHost(kit.WithModules(
		module("alpha", nil),
		module("beta", failure),
		module("gamma", nil),
	))
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err := host.Shutdown(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("Shutdown error = %v, want it to wrap %v", err, failure)
	}
	if !strings.Contains(err.Error(), "shut down module beta") {
		t.Fatalf("Shutdown error = %v, want it to name the module", err)
	}
	if want := []string{"gamma", "beta", "alpha"}; !reflect.DeepEqual(closed, want) {
		t.Fatalf("modules closed in %v, want %v", closed, want)
	}
}

// TestFailedInitReleasesWhatItAlreadyAcquired checks the startup path nothing
// else covers: the module that opened the pool came first, the one behind it
// failed, and the process exits without a listener. Without the teardown the
// pool is closed only by process exit, which is not a plan.
func TestFailedInitReleasesWhatItAlreadyAcquired(t *testing.T) {
	failure := errors.New("schema is wrong")
	var closed []string
	host := kit.MustNewHost(kit.WithModules(
		&closingModule{
			hookModule: &hookModule{name: "database", init: func(context.Context, *kit.Registry) error { return nil }},
			shutdown: func(context.Context, *kit.Registry) error {
				closed = append(closed, "database")
				return nil
			},
		},
		&hookModule{
			name:     "schema",
			requires: []string{"database"},
			init:     func(context.Context, *kit.Registry) error { return failure },
		},
	))
	err := host.Run(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("Run error = %v, want it to wrap %v", err, failure)
	}
	if want := []string{"database"}; !reflect.DeepEqual(closed, want) {
		t.Fatalf("modules closed = %v, want %v", closed, want)
	}
}

// TestModuleShutdownRunsOnce checks that the paths which all end in teardown — an
// explicit Shutdown called twice, and the one Run already ran — do not hand a
// pool back more than once.
func TestModuleShutdownRunsOnce(t *testing.T) {
	var closes int
	module := &closingModule{
		hookModule: &hookModule{name: "database", init: func(context.Context, *kit.Registry) error { return nil }},
		shutdown: func(context.Context, *kit.Registry) error {
			closes++
			return nil
		},
	}
	host := kit.MustNewHost(kit.WithModules(module))
	if err := host.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if closes != 1 {
		t.Fatalf("module shut down %d times, want 1", closes)
	}
}
