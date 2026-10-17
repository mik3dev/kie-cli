package api

import (
	"context"
	"testing"
	"time"
)

type fakeClock struct {
	t     time.Time
	slept time.Duration
}

func (f *fakeClock) now() time.Time { return f.t }
func (f *fakeClock) sleep(_ context.Context, d time.Duration) error {
	f.slept += d
	f.t = f.t.Add(d)
	return nil
}

func TestDiscoveryLimiterSpacing(t *testing.T) {
	fc := &fakeClock{t: time.Unix(1000, 0)}
	l := newLimiter(fc.now, fc.sleep)
	for i := 0; i < 3; i++ {
		if err := l.wait(context.Background(), ClassDiscovery); err != nil {
			t.Fatal(err)
		}
	}
	if fc.slept < 2200*time.Millisecond {
		t.Fatalf("slept %v, want >= 2.2s", fc.slept)
	}
}

func TestCreateLimiterSlidingWindow(t *testing.T) {
	fc := &fakeClock{t: time.Unix(1000, 0)}
	l := newLimiter(fc.now, fc.sleep)
	for i := 0; i < 20; i++ {
		_ = l.wait(context.Background(), ClassCreate)
	}
	if fc.slept != 0 {
		t.Fatalf("first 20 should not sleep, slept %v", fc.slept)
	}
	_ = l.wait(context.Background(), ClassCreate)
	if fc.slept != 10*time.Second {
		t.Fatalf("21st should wait window, slept %v", fc.slept)
	}
}

func TestRecordLimiterAndOther(t *testing.T) {
	fc := &fakeClock{t: time.Unix(1000, 0)}
	l := newLimiter(fc.now, fc.sleep)
	for i := 0; i < 50; i++ {
		_ = l.wait(context.Background(), ClassOther)
	}
	if fc.slept != 0 {
		t.Fatalf("other must not be limited")
	}
	for i := 0; i < 11; i++ {
		_ = l.wait(context.Background(), ClassRecord)
	}
	if fc.slept != time.Second {
		t.Fatalf("11th record should wait 1s, slept %v", fc.slept)
	}
}
