package configpush

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushQueue_SingleFlightPerAgent(t *testing.T) {
	t.Parallel()

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var calls atomic.Int32

	started := make(chan struct{})
	release := make(chan struct{})

	q := newPushQueue(func(ctx context.Context, agentID string) error {
		calls.Add(1)
		c := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		concurrent.Add(-1)
		return nil
	})

	q.Request("agent-1")
	q.Request("agent-1")
	q.Request("agent-1")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("push never started")
	}

	// Still held in first push; more Requests only set pending.
	q.Request("agent-1")
	assert.Equal(t, int32(1), maxConcurrent.Load())

	close(release)

	require.Eventually(t, func() bool {
		return calls.Load() == 2 // one in-flight + one coalesced follow-up
	}, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, int32(1), maxConcurrent.Load(), "never run two pushes concurrently for same agent")
}

func TestPushQueue_DifferentAgentsRunInParallel(t *testing.T) {
	t.Parallel()

	var gate sync.WaitGroup
	gate.Add(2)
	entered := make(chan string, 2)

	q := newPushQueue(func(ctx context.Context, agentID string) error {
		entered <- agentID
		gate.Done()
		gate.Wait() // both must enter before either finishes
		return nil
	})

	q.Request("a")
	q.Request("b")

	require.Eventually(t, func() bool {
		return len(entered) == 2
	}, 2*time.Second, 10*time.Millisecond)
}

func TestPushQueue_PushErrorStillClearsInFlight(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	q := newPushQueue(func(ctx context.Context, agentID string) error {
		calls.Add(1)
		return errors.New("push failed")
	})

	q.Request("agent-1")
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 5*time.Millisecond)

	q.Request("agent-1")
	require.Eventually(t, func() bool { return calls.Load() == 2 }, time.Second, 5*time.Millisecond)
}
