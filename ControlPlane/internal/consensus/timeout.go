package consensus

import "time"

// TimeoutScheduler manages consensus timeouts with exponential backoff.
// Per SPEC.md §10: "Timeout increases exponentially under repeated failures."
type TimeoutScheduler struct {
	baseTimeout    time.Duration
	maxTimeout     time.Duration
	lastCommitRound uint64
	timer          *time.Timer
}

// NewTimeoutScheduler creates a new TimeoutScheduler.
func NewTimeoutScheduler(baseMs, maxMs int64) *TimeoutScheduler {
	if baseMs <= 0 {
		baseMs = 3000
	}
	if maxMs <= 0 {
		maxMs = 60000
	}
	return &TimeoutScheduler{
		baseTimeout: time.Duration(baseMs) * time.Millisecond,
		maxTimeout:  time.Duration(maxMs) * time.Millisecond,
	}
}

// TimeoutDuration calculates timeout for a given round.
// Duration: baseTimeout * 2^(round - lastCommitRound), capped at maxTimeout.
func (ts *TimeoutScheduler) TimeoutDuration(round uint64) time.Duration {
	exponent := round
	if round > ts.lastCommitRound {
		exponent = round - ts.lastCommitRound
	}

	// Cap exponent to prevent overflow.
	if exponent > 20 {
		exponent = 20
	}

	d := ts.baseTimeout * (1 << exponent)
	if d > ts.maxTimeout {
		d = ts.maxTimeout
	}
	return d
}

// ScheduleTimeout starts a timeout for the given round.
// Returns a channel that fires when the timeout expires.
func (ts *TimeoutScheduler) ScheduleTimeout(round uint64) <-chan time.Time {
	d := ts.TimeoutDuration(round)
	if ts.timer != nil {
		ts.timer.Stop()
	}
	ts.timer = time.NewTimer(d)
	return ts.timer.C
}

// Reset resets the backoff after a successful commit at the given round.
func (ts *TimeoutScheduler) Reset(commitRound uint64) {
	ts.lastCommitRound = commitRound
	if ts.timer != nil {
		ts.timer.Stop()
		ts.timer = nil
	}
}

// Stop stops the current timer.
func (ts *TimeoutScheduler) Stop() {
	if ts.timer != nil {
		ts.timer.Stop()
		ts.timer = nil
	}
}
