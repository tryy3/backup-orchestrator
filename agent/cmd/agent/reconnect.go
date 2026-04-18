package main

import (
	"context"
	"log/slog"
	"time"
)

const (
	reconnectInitialBackoff   = time.Second
	reconnectMaxBackoff       = 5 * time.Minute
	reconnectHealthyThreshold = 30 * time.Second
)

// runReconnectLoop repeatedly calls runFn and reconnects with exponential backoff.
// The backoff resets to its initial value whenever runFn stays connected for at least
// reconnectHealthyThreshold — ensuring a long-lived, healthy connection always gets
// a fast reconnect after the next disconnect.
//
// sleep and now are injectable for tests; pass realSleep and time.Now in production.
func runReconnectLoop(
	ctx context.Context,
	runFn func(context.Context) error,
	flushFn func(),
	sleep func(ctx context.Context, d time.Duration),
	now func() time.Time,
) {
	backoff := reconnectInitialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		slog.Info("connecting to server...", "source", "agent")
		start := now()
		err := runFn(ctx)
		if ctx.Err() != nil {
			return // context cancelled, shutting down
		}

		// Reset backoff if the connection was healthy long enough.
		if now().Sub(start) >= reconnectHealthyThreshold {
			backoff = reconnectInitialBackoff
		}

		slog.Warn("stream disconnected", "source", "agent", "error", err)
		flushFn()

		slog.Info("reconnecting", "source", "agent", "backoff", backoff)
		sleep(ctx, backoff)

		// Exponential backoff with cap.
		backoff = min(backoff*2, reconnectMaxBackoff)
	}
}

// realSleep sleeps for duration d, but returns early if ctx is cancelled.
func realSleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
