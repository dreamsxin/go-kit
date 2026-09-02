package endpointer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

func labelled(address string, labels map[string]any) sd.Instance {
	return sd.Instance{Address: address, Metadata: labels}
}

func newLabelledSet(t *testing.T, instances ...sd.Instance) endpointer.InstanceEndpointer {
	t.Helper()
	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, echoFactory, nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	cache.Update(sd.Event{Instances: instances})
	time.Sleep(20 * time.Millisecond)
	return set
}

func addressesOf(t *testing.T, set endpointer.InstanceEndpointer) []string {
	t.Helper()
	instances, err := set.InstanceEndpoints()
	if err != nil {
		t.Fatalf("InstanceEndpoints: %v", err)
	}
	addresses := make([]string, len(instances))
	for i, item := range instances {
		addresses[i] = item.Address()
	}
	return addresses
}

func TestFilter_KeepsMatchingInstances(t *testing.T) {
	set := newLabelledSet(t,
		labelled("north-1:80", map[string]any{"zone": "north"}),
		labelled("north-2:80", map[string]any{"zone": "north"}),
		labelled("south-1:80", map[string]any{"zone": "south"}),
	)

	local := endpointer.Filter(set, sd.MetadataEquals("zone", "north"))
	got := addressesOf(t, local)
	if len(got) != 2 || got[0] != "north-1:80" || got[1] != "north-2:80" {
		t.Fatalf("subset = %v, want the two north instances", got)
	}
}

// The strict policy fails rather than sending the request somewhere the caller
// ruled out, and a balancer reports that as ErrNoEndpoints.
func TestFilter_EmptyMatchYieldsNoEndpoints(t *testing.T) {
	set := newLabelledSet(t, labelled("south-1:80", map[string]any{"zone": "south"}))

	local := endpointer.Filter(set, sd.MetadataEquals("zone", "north"))
	if got := addressesOf(t, local); len(got) != 0 {
		t.Fatalf("subset = %v, want empty", got)
	}
	if _, err := balancer.NewRoundRobin(local).Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

func TestPrefer_FallsBackToFullSet(t *testing.T) {
	set := newLabelledSet(t,
		labelled("south-1:80", map[string]any{"zone": "south"}),
		labelled("south-2:80", map[string]any{"zone": "south"}),
	)

	local := endpointer.Prefer(set, sd.MetadataEquals("zone", "north"))
	got := addressesOf(t, local)
	if len(got) != 2 {
		t.Fatalf("subset = %v, want the full set as fallback", got)
	}
}

func TestPrefer_StaysLocalWhenLocalExists(t *testing.T) {
	set := newLabelledSet(t,
		labelled("north-1:80", map[string]any{"zone": "north"}),
		labelled("south-1:80", map[string]any{"zone": "south"}),
	)

	local := endpointer.Prefer(set, sd.MetadataEquals("zone", "north"))
	got := addressesOf(t, local)
	if len(got) != 1 || got[0] != "north-1:80" {
		t.Fatalf("subset = %v, want only the north instance", got)
	}
}

// Consul hands labels over as strings, so an int literal in the predicate must
// still match. Without the coercion this silently filters everything out.
func TestMetadataEquals_CoercesRegistryStrings(t *testing.T) {
	set := newLabelledSet(t, labelled("svc:80", map[string]any{"weight": "10"}))

	matched := endpointer.Filter(set, sd.MetadataEquals("weight", 10))
	if got := addressesOf(t, matched); len(got) != 1 {
		t.Fatalf("subset = %v, want the instance whose registry weight is \"10\"", got)
	}
}

func TestFilter_EndpointsMirrorsInstanceEndpoints(t *testing.T) {
	set := newLabelledSet(t,
		labelled("north:80", map[string]any{"zone": "north"}),
		labelled("south:80", map[string]any{"zone": "south"}),
	)

	local := endpointer.Filter(set, sd.MetadataEquals("zone", "north"))
	endpoints, err := local.Endpoints()
	if err != nil {
		t.Fatalf("Endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("Endpoints returned %d, want 1", len(endpoints))
	}
}

func TestFilter_ClosesUnderlyingSet(t *testing.T) {
	set := newLabelledSet(t, labelled("svc:80", nil))
	local := endpointer.Filter(set, sd.HasMetadata("zone"))

	if err := local.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := set.InstanceEndpoints(); !errors.Is(err, endpointer.ErrCacheClosed) {
		t.Fatalf("underlying set error = %v, want ErrCacheClosed", err)
	}
}

func TestFilter_PropagatesSourceError(t *testing.T) {
	set := newLabelledSet(t, labelled("svc:80", nil))
	if err := set.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	local := endpointer.Filter(set, sd.HasMetadata("zone"))
	if _, err := local.InstanceEndpoints(); !errors.Is(err, endpointer.ErrCacheClosed) {
		t.Fatalf("InstanceEndpoints error = %v, want ErrCacheClosed", err)
	}
	if _, err := local.Endpoints(); !errors.Is(err, endpointer.ErrCacheClosed) {
		t.Fatalf("Endpoints error = %v, want ErrCacheClosed", err)
	}
}

func TestFilter_NilMatchPanics(t *testing.T) {
	set := newLabelledSet(t, labelled("svc:80", nil))

	for name, build := range map[string]func(){
		"Subset":       func() { endpointer.Filter(set, nil) },
		"PreferSubset": func() { endpointer.Prefer(set, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic on a nil match")
				}
			}()
			build()
		})
	}
}
