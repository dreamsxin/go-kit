package endpointer

import (
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
)

// Filter narrows an endpoint set to the instances a match accepts. Selection
// strategies compose on top, so zone-aware round robin is Filter plus
// NewRoundRobin rather than a third strategy. It is the endpoint-layer twin of
// selector.Filter, and the same predicates drive both.
//
// When nothing matches, the set is empty and balancers report
// sd.ErrNoEndpoints. That is the strict policy — Envoy's NO_FALLBACK — and it
// is deliberate: better to fail than to send a request somewhere the caller
// ruled out. Use Prefer when spilling over is preferable to failing.
//
// Predicates come from the root sd package: sd.MetadataEquals, sd.MetadataIn,
// sd.MetadataMatches, sd.HasMetadata, and sd.And / sd.Or / sd.Not.
func Filter(source InstanceEndpointer, match sd.Match) InstanceEndpointer {
	if match == nil {
		panic("endpointer: nil filter match")
	}
	return &filtered{source: source, match: match}
}

// Prefer narrows an endpoint set but falls back to the full set when nothing
// matches — Envoy's ANY_ENDPOINT. This is how zone-aware routing degrades:
// stay local while local instances exist, spill over to other zones rather
// than fail.
func Prefer(source InstanceEndpointer, match sd.Match) InstanceEndpointer {
	if match == nil {
		panic("endpointer: nil filter match")
	}
	return &filtered{source: source, match: match, fallback: true}
}

type filtered struct {
	source   InstanceEndpointer
	match    sd.Match
	fallback bool
}

func (s *filtered) Close() error { return s.source.Close() }

func (s *filtered) Endpoints() ([]endpoint.Endpoint, error) {
	instances, err := s.InstanceEndpoints()
	if err != nil {
		return nil, err
	}
	endpoints := make([]endpoint.Endpoint, len(instances))
	for i, item := range instances {
		endpoints[i] = item.Endpoint
	}
	return endpoints, nil
}

func (s *filtered) InstanceEndpoints() ([]InstanceEndpoint, error) {
	instances, err := s.source.InstanceEndpoints()
	if err != nil {
		return nil, err
	}

	matched := make([]InstanceEndpoint, 0, len(instances))
	for _, item := range instances {
		if s.match(item.Instance) {
			matched = append(matched, item)
		}
	}
	if len(matched) == 0 && s.fallback {
		return instances, nil
	}
	return matched, nil
}
