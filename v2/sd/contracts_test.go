package sd_test

import (
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
)

func TestEvent(t *testing.T) {
	sentinel := errors.New("discovery unavailable")
	event := sd.Event{Instances: []string{"a:80", "b:80"}, Err: sentinel}
	if len(event.Instances) != 2 || !errors.Is(event.Err, sentinel) {
		t.Fatalf("unexpected event: %+v", event)
	}
}
