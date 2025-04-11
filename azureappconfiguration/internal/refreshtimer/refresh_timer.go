// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package refreshtimer

import "time"

// RefreshTimer manages the timing for refresh operations
type RefreshTimer struct {
	interval        time.Duration // How often refreshes should occur
	nextRefreshTime time.Time     // When the next refresh should occur
}

// RefreshCondition interface defines the methods a refresh timer should implement
type RefreshCondition interface {
	ShouldRefresh() bool
	Reset()
}

const (
	DefaultRefreshInterval time.Duration = 30 * time.Second
)

// New creates a new refresh timer with the specified interval
// If interval is zero or negative, it falls back to the DefaultRefreshInterval
func New(interval time.Duration) *RefreshTimer {
	// Use default interval if not specified or invalid
	if interval <= 0 {
		interval = DefaultRefreshInterval
	}

	return &RefreshTimer{
		interval:        interval,
		nextRefreshTime: time.Now().Add(interval),
	}
}

// ShouldRefresh checks whether it's time for a refresh
func (rt *RefreshTimer) ShouldRefresh() bool {
	return !time.Now().Before(rt.nextRefreshTime)
}

// Reset resets the timer for the next refresh cycle
func (rt *RefreshTimer) Reset() {
	rt.nextRefreshTime = time.Now().Add(rt.interval)
}
