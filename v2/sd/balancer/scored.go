package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// ScoreFunc rates an instance; the highest score wins, and false excludes the
// instance entirely.
type ScoreFunc = selector.ScoreFunc

// NewScored selects the highest-scoring endpoint, breaking ties at random.
//
// This is the seam for load signals the process did not measure itself: a
// report pushed by the instances, ORCA or LRS style out-of-band reporting, or
// any table the caller keeps. Such a signal is stale by at least one reporting
// interval — that is inherent to the channel, not a defect. When this process
// is on the data path and can observe the calls, feedback.Table.LeastRequest
// measures the truth instead.
//
// NewScored panics on a nil score function, which is a programming error rather
// than a runtime condition.
func NewScored(source endpointer.InstanceEndpointer, score ScoreFunc) sd.Balancer {
	return New(source, selector.Scored(score))
}
