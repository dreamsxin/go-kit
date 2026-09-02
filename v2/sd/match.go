package sd

import "context"

// Match reports whether an instance belongs to a subset. Predicates read
// labels, so they live next to the metadata readers rather than in one
// consuming package: sd/endpointer filters endpoint sets with them and
// sd/selector filters instance snapshots with them.
//
// A Match is evaluated on every selection, so keep it cheap and free of I/O.
type Match func(Instance) bool

// MetadataEquals matches instances whose label equals value.
//
// Comparison is on the string form of both sides, because a registry delivers
// every label as a string while callers write typed literals: matching on
// weight 10 therefore also matches the "10" Consul returned.
func MetadataEquals(key string, value any) Match {
	want, _ := MetadataString(map[string]any{key: value}, key)
	return func(instance Instance) bool {
		got, ok := MetadataString(instance.Metadata, key)
		return ok && got == want
	}
}

// MetadataIn matches instances whose label equals any of values.
func MetadataIn(key string, values ...any) Match {
	wanted := make(map[string]struct{}, len(values))
	for _, value := range values {
		text, _ := MetadataString(map[string]any{key: value}, key)
		wanted[text] = struct{}{}
	}
	return func(instance Instance) bool {
		got, ok := MetadataString(instance.Metadata, key)
		if !ok {
			return false
		}
		_, found := wanted[got]
		return found
	}
}

// MetadataMatches matches instances carrying every one of the given labels.
// An empty label set matches everything.
func MetadataMatches(labels map[string]any) Match {
	matches := make([]Match, 0, len(labels))
	for key, value := range labels {
		matches = append(matches, MetadataEquals(key, value))
	}
	return And(matches...)
}

// HasMetadata matches instances that carry key at all, whatever its value.
func HasMetadata(key string) Match {
	return func(instance Instance) bool {
		_, ok := instance.Metadata[key]
		return ok
	}
}

// And matches instances accepted by every match. No matches accepts everything.
func And(matches ...Match) Match {
	return func(instance Instance) bool {
		for _, match := range matches {
			if !match(instance) {
				return false
			}
		}
		return true
	}
}

// Or matches instances accepted by at least one match. No matches accepts
// nothing, which keeps Or(nil...) from silently widening a subset.
func Or(matches ...Match) Match {
	return func(instance Instance) bool {
		for _, match := range matches {
			if match(instance) {
				return true
			}
		}
		return false
	}
}

// Not inverts a match.
func Not(match Match) Match {
	return func(instance Instance) bool { return !match(instance) }
}

// InstanceFilter narrows a candidate set before selection. It exists next to
// Match because some policies cannot be expressed per instance: passive health
// checking has to know how much of the pool it is about to remove before it
// removes any of it, which is a decision about the set, not about one member.
//
// Implementations must not modify the slice they receive; they return the
// candidates to keep, in any order. Returning an empty slice means "nothing is
// selectable", which surfaces to the caller as ErrNoEndpoints.
type InstanceFilter func(ctx context.Context, instances []Instance) []Instance

// Keep lifts a per-instance predicate into an InstanceFilter, so static label
// matching and set-level policies compose in one list.
func Keep(match Match) InstanceFilter {
	if match == nil {
		panic("sd: nil match")
	}
	return func(_ context.Context, instances []Instance) []Instance {
		kept := make([]Instance, 0, len(instances))
		for _, instance := range instances {
			if match(instance) {
				kept = append(kept, instance)
			}
		}
		return kept
	}
}

// Conventional metadata for lifecycle state. Every registry this project
// integrates with can carry it — Consul meta, etcd JSON, Nacos metadata,
// Kubernetes labels — so agreeing on one key is what makes a draining instance
// mean the same thing to every consumer.
const (
	StateKey      = "state"
	StateReady    = "ready"
	StateDraining = "draining"
)

// Draining matches instances that asked to stop receiving new work.
//
// Only new selections are affected. Whatever already holds a connection to a
// draining instance keeps it: sd does not own connections, so it cannot end them
// and does not pretend to.
func Draining() Match {
	return MetadataEquals(StateKey, StateDraining)
}

// Serving matches instances that are not draining, which is the filter almost
// every caller wants. An instance with no state label is serving: a registry
// that never sets one must not go dark.
func Serving() Match {
	return Not(Draining())
}
