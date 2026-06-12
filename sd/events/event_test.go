package events

import (
	"errors"
	"testing"
)

func TestEvent_DefaultIsHealthy(t *testing.T) {
	e := Event{
		Instances: []string{"host1:8080", "host2:8080"},
	}
	if e.Err != nil {
		t.Errorf("expected nil Err, got %v", e.Err)
	}
	if len(e.Instances) != 2 {
		t.Errorf("expected 2 instances, got %d", len(e.Instances))
	}
}

func TestEvent_WithError(t *testing.T) {
	err := errors.New("consul down")
	e := Event{Err: err}
	if !errors.Is(e.Err, err) {
		t.Errorf("expected Err to be %v, got %v", err, e.Err)
	}
	if len(e.Instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(e.Instances))
	}
}

func TestEvent_EmptyInstances(t *testing.T) {
	e := Event{}
	if e.Instances != nil {
		t.Errorf("expected nil Instances, got %v", e.Instances)
	}
}
