package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClock is a manually-advanced clock for use in tests.
type fakeClock struct {
	t time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Now()}
}

func (c *fakeClock) Now() time.Time {
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

// recordingSleep records each sleep duration for assertion in tests.
func recordingSleep(durations *[]time.Duration) func(context.Context, time.Duration) {
	return func(_ context.Context, d time.Duration) {
		*durations = append(*durations, d)
	}
}

// TestRunReconnectLoop_BackoffEscalatesOnImmediateFailures verifies that each
// consecutive immediate failure doubles the backoff duration.
func TestRunReconnectLoop_BackoffEscalatesOnImmediateFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clock := newFakeClock()
	var slept []time.Duration

	calls := 0
	runFn := func(ctx context.Context) error {
		calls++
		if calls >= 4 {
			cancel()
		}
		return errors.New("connection refused")
	}

	runReconnectLoop(ctx, runFn, func() {}, recordingSleep(&slept), clock.Now)

	// Expect escalation: 1 s, 2 s, 4 s (three failures before cancel on call 4).
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(slept) != len(want) {
		t.Fatalf("expected %d sleep calls, got %d: %v", len(want), len(slept), slept)
	}
	for i, d := range want {
		if slept[i] != d {
			t.Errorf("sleep[%d]: want %v, got %v", i, d, slept[i])
		}
	}
}

// TestRunReconnectLoop_BackoffResetsAfterHealthyConnection verifies that when
// a connection stays up for at least reconnectHealthyThreshold the backoff
// resets to the initial value, giving the next disconnect a fast reconnect.
func TestRunReconnectLoop_BackoffResetsAfterHealthyConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clock := newFakeClock()
	var slept []time.Duration

	calls := 0
	runFn := func(ctx context.Context) error {
		calls++
		switch calls {
		case 1, 2:
			// Immediate failure — escalates backoff.
			return errors.New("connection refused")
		case 3:
			// Healthy connection: advance the fake clock by the healthy threshold.
			clock.Advance(reconnectHealthyThreshold)
			return errors.New("disconnected after healthy run")
		default:
			cancel()
			return errors.New("connection refused")
		}
	}

	runReconnectLoop(ctx, runFn, func() {}, recordingSleep(&slept), clock.Now)

	// After call 1 (immediate fail): sleep 1 s, backoff → 2 s.
	// After call 2 (immediate fail): sleep 2 s, backoff → 4 s.
	// After call 3 (healthy):        reset to 1 s, sleep 1 s, backoff → 2 s.
	want := []time.Duration{time.Second, 2 * time.Second, time.Second}
	if len(slept) != len(want) {
		t.Fatalf("expected %d sleep calls, got %d: %v", len(want), len(slept), slept)
	}
	for i, d := range want {
		if slept[i] != d {
			t.Errorf("sleep[%d]: want %v, got %v", i, d, slept[i])
		}
	}
}

// TestRunReconnectLoop_BackoffCapsAtMax verifies that the backoff never exceeds
// reconnectMaxBackoff regardless of how many consecutive failures occur.
func TestRunReconnectLoop_BackoffCapsAtMax(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clock := newFakeClock()
	var slept []time.Duration

	calls := 0
	runFn := func(ctx context.Context) error {
		calls++
		if calls > 10 {
			cancel()
		}
		return errors.New("fail")
	}

	runReconnectLoop(ctx, runFn, func() {}, recordingSleep(&slept), clock.Now)

	for _, d := range slept {
		if d > reconnectMaxBackoff {
			t.Errorf("backoff %v exceeds max %v", d, reconnectMaxBackoff)
		}
	}

	// After enough failures the last sleep should be exactly the cap.
	if len(slept) == 0 {
		t.Fatal("expected at least one sleep call")
	}
	last := slept[len(slept)-1]
	if last != reconnectMaxBackoff {
		t.Errorf("expected last sleep to equal maxBackoff %v, got %v", reconnectMaxBackoff, last)
	}
}

// TestRunReconnectLoop_StopsOnContextCancel verifies that the loop exits
// immediately when the context is cancelled before the first Run call.
func TestRunReconnectLoop_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the loop starts

	clock := newFakeClock()
	var slept []time.Duration

	runFn := func(ctx context.Context) error {
		t.Error("runFn should not be called when context is already cancelled")
		return nil
	}

	runReconnectLoop(ctx, runFn, func() {}, recordingSleep(&slept), clock.Now)

	if len(slept) != 0 {
		t.Errorf("expected no sleeps, got %d: %v", len(slept), slept)
	}
}
