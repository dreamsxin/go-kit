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
// interval — that is inherent to the channel, not a defect. It needs no
// accounting of its own, which is why it belongs here rather than in
// sd/feedback: nothing in this process has to observe the calls for the score to
// mean something.
//
// When this process is on the data path and can observe the calls,
// sd/feedback.Measure assembles the measured strategies — Scored, LeastRequest,
// SlowStartWeighted — together with the accounting that feeds them.
//
// NewScored panics on a nil score function, which is a programming error rather
// than a runtime condition.
func NewScored(source endpointer.InstanceEndpointer, score ScoreFunc) sd.Balancer {
	return New(source, selector.Scored(score))
}
