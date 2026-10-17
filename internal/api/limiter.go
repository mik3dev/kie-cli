package api

import (
	"context"
	"sync"
	"time"
)

// Class groups endpoints that share a rate limit.
type Class int

const (
	ClassOther Class = iota
	ClassDiscovery
	ClassCreate
	ClassRecord
)

const (
	discoveryGap = 1100 * time.Millisecond
	createMax    = 20
	createWindow = 10 * time.Second
	recordMax    = 10
	recordWindow = time.Second
)

// ponytail: in-process only; parallel kie-cli processes don't coordinate, add file lock if 429s show up.
type limiter struct {
	mu    sync.Mutex
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
	slots map[Class][]time.Time // scheduled start times per class
}

func newLimiter(now func() time.Time, sleep func(context.Context, time.Duration) error) *limiter {
	return &limiter{now: now, sleep: sleep, slots: map[Class][]time.Time{}}
}

// wait reserves the next slot for the class and sleeps until it.
func (l *limiter) wait(ctx context.Context, c Class) error {
	var max int
	var window time.Duration
	switch c {
	case ClassDiscovery:
		max, window = 1, discoveryGap
	case ClassCreate:
		max, window = createMax, createWindow
	case ClassRecord:
		max, window = recordMax, recordWindow
	default:
		return nil
	}
	l.mu.Lock()
	now := l.now()
	at := now
	s := l.slots[c]
	if len(s) >= max {
		if earliest := s[len(s)-max].Add(window); earliest.After(at) {
			at = earliest
		}
	}
	if len(s) >= max {
		s = s[len(s)-max+1:]
	}
	l.slots[c] = append(append([]time.Time(nil), s...), at)
	l.mu.Unlock()
	if d := at.Sub(now); d > 0 {
		return l.sleep(ctx, d)
	}
	return nil
}
