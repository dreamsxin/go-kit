package sd_test

import (
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
)

func TestMatchPredicates(t *testing.T) {
	north := sd.Instance{Address: "north:80", Metadata: map[string]any{"zone": "north", "version": "v2", "tls": true}}
	south := sd.Instance{Address: "south:80", Metadata: map[string]any{"zone": "south", "version": "v1"}}

	tests := []struct {
		name        string
		match       sd.Match
		northWanted bool
		southWanted bool
	}{
		{name: "equals", match: sd.MetadataEquals("zone", "north"), northWanted: true},
		{name: "in", match: sd.MetadataIn("zone", "north", "south"), northWanted: true, southWanted: true},
		{name: "in misses", match: sd.MetadataIn("zone", "east")},
		{name: "matches all labels", match: sd.MetadataMatches(map[string]any{"zone": "north", "version": "v2"}), northWanted: true},
		{name: "matches rejects partial", match: sd.MetadataMatches(map[string]any{"zone": "north", "version": "v1"})},
		{name: "empty label set matches all", match: sd.MetadataMatches(nil), northWanted: true, southWanted: true},
		{name: "has key", match: sd.HasMetadata("tls"), northWanted: true},
		{name: "not", match: sd.Not(sd.MetadataEquals("zone", "north")), southWanted: true},
		{name: "and", match: sd.And(sd.MetadataEquals("zone", "north"), sd.HasMetadata("tls")), northWanted: true},
		{name: "or", match: sd.Or(sd.MetadataEquals("zone", "north"), sd.MetadataEquals("zone", "south")), northWanted: true, southWanted: true},
		{name: "and with no terms matches all", match: sd.And(), northWanted: true, southWanted: true},
		{name: "or with no terms matches none", match: sd.Or()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.match(north); got != tt.northWanted {
				t.Errorf("north matched = %v, want %v", got, tt.northWanted)
			}
			if got := tt.match(south); got != tt.southWanted {
				t.Errorf("south matched = %v, want %v", got, tt.southWanted)
			}
		})
	}
}

// A registry delivers every label as a string, so a predicate written against
// a typed literal must still match. Without the coercion this silently filters
// everything out.
func TestMetadataEquals_CoercesRegistryStrings(t *testing.T) {
	instance := sd.Instance{Address: "svc:80", Metadata: map[string]any{"weight": "10"}}

	if !sd.MetadataEquals("weight", 10)(instance) {
		t.Error("int literal 10 did not match the registry string \"10\"")
	}
	if !sd.MetadataIn("weight", 5, 10)(instance) {
		t.Error("int literal in set did not match the registry string \"10\"")
	}
}

func TestMatch_MissingLabelNeverMatches(t *testing.T) {
	bare := sd.Instance{Address: "svc:80"}

	for name, match := range map[string]sd.Match{
		"equals":  sd.MetadataEquals("zone", "north"),
		"in":      sd.MetadataIn("zone", "north"),
		"matches": sd.MetadataMatches(map[string]any{"zone": "north"}),
		"has":     sd.HasMetadata("zone"),
	} {
		if match(bare) {
			t.Errorf("%s matched an instance with no labels", name)
		}
	}
}
