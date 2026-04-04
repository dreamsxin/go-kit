package events

// Event is broadcast by an Instancer whenever the set of healthy service
// instances changes.  If Err is non-nil the instance list should be treated
// as stale; the previous list remains valid until InvalidateOnError expires.
type Event struct {
	Instances []string
	Err       error
}
