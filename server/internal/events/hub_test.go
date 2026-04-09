package events

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_RegisterUnregister(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	id, ch := hub.Register()
	assert.NotEmpty(t, id)
	assert.NotNil(t, ch)
	assert.Equal(t, 1, hub.ClientCount())

	hub.Unregister(id)
	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_RegisterMultiple(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	id1, _ := hub.Register()
	id2, _ := hub.Register()
	id3, _ := hub.Register()

	assert.NotEqual(t, id1, id2)
	assert.NotEqual(t, id2, id3)
	assert.Equal(t, 3, hub.ClientCount())
}

func TestHub_UnregisterUnknown(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	// Should not panic.
	hub.Unregister("nonexistent")
	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_UnregisterIdempotent(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	id, _ := hub.Register()
	hub.Unregister(id)
	hub.Unregister(id) // Should not panic on double unregister.
	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_Broadcast(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	_, ch1 := hub.Register()
	_, ch2 := hub.Register()

	event := Event{Type: "test.event", Payload: map[string]string{"key": "value"}}
	hub.Broadcast(event)

	// Both clients should receive the event.
	select {
	case data := <-ch1:
		var received Event
		err := json.Unmarshal(data, &received)
		require.NoError(t, err)
		assert.Equal(t, "test.event", received.Type)
	case <-time.After(time.Second):
		t.Fatal("client 1 did not receive event")
	}

	select {
	case data := <-ch2:
		var received Event
		err := json.Unmarshal(data, &received)
		require.NoError(t, err)
		assert.Equal(t, "test.event", received.Type)
	case <-time.After(time.Second):
		t.Fatal("client 2 did not receive event")
	}
}

func TestHub_BroadcastDropsFullBuffer(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	_, ch := hub.Register()

	// Fill the channel buffer (capacity is 64).
	for i := 0; i < 64; i++ {
		hub.Broadcast(Event{Type: "filler"})
	}

	// This broadcast should be dropped (non-blocking), not block.
	done := make(chan struct{})
	go func() {
		hub.Broadcast(Event{Type: "overflow"})
		close(done)
	}()

	select {
	case <-done:
		// Good, Broadcast returned without blocking.
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked on full buffer")
	}

	// The channel should have exactly 64 items.
	assert.Len(t, ch, 64)
}

func TestHub_Close(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	id, ch := hub.Register()

	hub.Close()

	// Client count should be 0 after close.
	assert.Equal(t, 0, hub.ClientCount())

	// Channel should be closed.
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed")

	// Unregister after close should not panic.
	hub.Unregister(id)
}

func TestHub_CloseIdempotent(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	hub.Register()

	hub.Close()
	hub.Close() // Should not panic.
}

func TestHub_BroadcastAfterClose(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	hub.Register()
	hub.Close()

	// Should not panic.
	hub.Broadcast(Event{Type: "after.close"})
}

func TestHub_ConcurrentBroadcast(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	_, ch := hub.Register()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			hub.Broadcast(Event{Type: "concurrent"})
		}(i)
	}
	wg.Wait()

	// Drain the channel and count events.
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	assert.Equal(t, 10, count)
}

func TestHub_ConcurrentRegisterUnregister(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _ := hub.Register()
			hub.Broadcast(Event{Type: "test"})
			hub.Unregister(id)
		}()
	}
	wg.Wait()

	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_UnregisterClosesChannel(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	defer hub.Close()

	_, ch := hub.Register()

	// Read from the channel in a goroutine.
	done := make(chan struct{})
	go func() {
		for range ch {
			// drain
		}
		close(done)
	}()

	// Unregister should close the channel, unblocking the reader.
	hub.Unregister(hub.clientIDs()[0])

	select {
	case <-done:
		// Good, channel was closed.
	case <-time.After(time.Second):
		t.Fatal("channel was not closed by Unregister")
	}
}

// clientIDs returns a snapshot of all client IDs (test helper).
func (h *Hub) clientIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}
