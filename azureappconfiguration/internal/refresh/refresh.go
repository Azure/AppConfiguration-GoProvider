// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package refresh

import "time"

// Timer manages the timing for refresh operations
type Timer struct {
	interval        time.Duration // How often refreshes should occur
	nextRefreshTime time.Time     // When the next refresh should occur
}

// Condition interface defines the methods a refresh timer should implement
type Condition interface {
	ShouldRefresh() bool
	Reset()
}

const (
	DefaultRefreshInterval time.Duration = 30 * time.Second
)

// New creates a new refresh timer with the specified interval
// If interval is zero or negative, it falls back to the DefaultRefreshInterval
func New(interval time.Duration) *Timer {
	// Use default interval if not specified or invalid
	if interval <= 0 {
		interval = DefaultRefreshInterval
	}

	return &Timer{
		interval:        interval,
		nextRefreshTime: time.Now().Add(interval),
	}
}

// ShouldRefresh checks whether it's time for a refresh
func (rt *Timer) ShouldRefresh() bool {
	return !time.Now().Before(rt.nextRefreshTime)
}

// Reset resets the timer for the next refresh cycle
func (rt *Timer) Reset() {
	rt.nextRefreshTime = time.Now().Add(rt.interval)
}
