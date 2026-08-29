package configpush

import (
	"context"
	"log/slog"
	"sync"
)

// pushFunc builds and sends config for one agent. Used so tests can inject a fake.
type pushFunc func(ctx context.Context, agentID string) error

type agentPushState struct {
	inFlight bool
	pending  bool
}

// pushQueue serializes config pushes per agent and coalesces bursts.
type pushQueue struct {
	mu     sync.Mutex
	agents map[string]*agentPushState
	push   pushFunc
}

func newPushQueue(push pushFunc) *pushQueue {
	return &pushQueue{
		agents: make(map[string]*agentPushState),
		push:   push,
	}
}

// Request schedules a config push for agentID. Non-blocking.
func (q *pushQueue) Request(agentID string) {
	q.mu.Lock()
	st := q.agents[agentID]
	if st == nil {
		st = &agentPushState{}
		q.agents[agentID] = st
	}
	if st.inFlight {
		st.pending = true
		q.mu.Unlock()
		return
	}
	st.inFlight = true
	q.mu.Unlock()

	go q.worker(agentID)
}

func (q *pushQueue) worker(agentID string) {
	for {
		q.mu.Lock()
		st := q.agents[agentID]
		st.pending = false
		q.mu.Unlock()

		if err := q.push(context.Background(), agentID); err != nil {
			slog.Error("config push failed", "agent_id", agentID, "error", err)
		}

		q.mu.Lock()
		st = q.agents[agentID]
		if st.pending {
			q.mu.Unlock()
			continue
		}
		st.inFlight = false
		q.mu.Unlock()
		return
	}
}
