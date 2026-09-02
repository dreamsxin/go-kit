package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// LoadFunc reports the current load of an instance; lower is better.
type LoadFunc = selector.LoadFunc

// LeastRequestOption configures NewLeastRequest.
type LeastRequestOption = selector.LeastRequestOption

// DefaultChoices is how many candidates a least-request selection samples.
const DefaultChoices = selector.DefaultChoices

// WithChoices sets how many candidates each selection samples.
func WithChoices(choices int) LeastRequestOption {
	return selector.WithChoices(choices)
}

// NewLeastRequest sends each call to the sampled instance with the fewest calls
// still in flight, and records the result of that call in table.
//
// Pass the table you already use for scoring or passive health checks so every
// policy reads one measurement stream; pass nil to give this balancer a private
// one. A shared table should follow the discovery snapshot — see
// feedback.Table.Follow — so that measurements for replaced instances are
// dropped.
//
// This is in-path measurement: the count is the truth as of this call, not a
// number an instance reported some interval ago. NewScored is the seam for
// signals that arrive out of band.
func NewLeastRequest(source endpointer.InstanceEndpointer, table *feedback.Table, options ...LeastRequestOption) sd.Balancer {
	if table == nil {
		table = feedback.NewTable()
	}
	return New(source, table.LeastRequest(options...))
}
