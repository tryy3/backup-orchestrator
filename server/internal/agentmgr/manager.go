package agentmgr

import (
	"fmt"
	"sync"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/google/uuid"
)

// ConnectedAgent represents a currently connected agent and its communication channels.
type ConnectedAgent struct {
	AgentID     string
	SendCh      chan *backupv1.ServerMessage
	LastHeart   time.Time
	Status      string
	CurrentJob  *CurrentJobInfo                         // tracks the currently running job (from heartbeats)
	PendingCmds map[string]chan *backupv1.CommandResult // command_id -> response channel
	mu          sync.Mutex
}

// CurrentJobInfo holds info about a job currently being executed by an agent.
type CurrentJobInfo struct {
	PlanName        string
	PlanID          string
	ProgressPercent float32
	StartedAt       string
}

// Manager maintains a thread-safe registry of connected agents.
type Manager struct {
	mu     sync.RWMutex
	agents map[string]*ConnectedAgent
	done   chan struct{}
	once   sync.Once
}

// New creates a new agent manager.
func New() *Manager {
	return &Manager{
		agents: make(map[string]*ConnectedAgent),
		done:   make(chan struct{}),
	}
}

// Register adds a connected agent to the registry.
func (m *Manager) Register(agentID string, sendCh chan *backupv1.ServerMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agentID] = &ConnectedAgent{
		AgentID:     agentID,
		SendCh:      sendCh,
		LastHeart:   time.Now(),
		Status:      "connected",
		PendingCmds: make(map[string]chan *backupv1.CommandResult),
	}
}

// Unregister removes a connected agent from the registry and closes its send channel.
func (m *Manager) Unregister(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if agent, ok := m.agents[agentID]; ok {
		// Close all pending command channels so waiters don't block forever.
		agent.mu.Lock()
		for id, ch := range agent.PendingCmds {
			close(ch)
			delete(agent.PendingCmds, id)
		}
		agent.mu.Unlock()

		// Close the send channel. This is safe because we hold the write lock,
		// which prevents any concurrent Send/SendCommand from accessing it.
		close(agent.SendCh)
		delete(m.agents, agentID)
	}
}

// Send sends a server message to a connected agent. Returns error if agent is not connected.
func (m *Manager) Send(agentID string, msg *backupv1.ServerMessage) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	select {
	case agent.SendCh <- msg:
		return nil
	default:
		return fmt.Errorf("agent %s send buffer full", agentID)
	}
}

// SendCommand sends a command to an agent and waits for the response with a 30-second timeout.
func (m *Manager) SendCommand(agentID string, cmd *backupv1.Command) (*backupv1.CommandResult, error) {
	// Assign a command ID if not set.
	if cmd.CommandId == "" {
		cmd.CommandId = uuid.New().String()
	}

	// Create a response channel.
	responseCh := make(chan *backupv1.CommandResult, 1)

	// Register pending command and send under read lock to prevent race with Unregister.
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("agent %s not connected", agentID)
	}

	agent.mu.Lock()
	agent.PendingCmds[cmd.CommandId] = responseCh
	agent.mu.Unlock()

	msg := &backupv1.ServerMessage{
		Payload: &backupv1.ServerMessage_Command{
			Command: cmd,
		},
	}
	select {
	case agent.SendCh <- msg:
	default:
		m.mu.RUnlock()
		agent.mu.Lock()
		delete(agent.PendingCmds, cmd.CommandId)
		agent.mu.Unlock()
		return nil, fmt.Errorf("agent %s send buffer full", agentID)
	}
	m.mu.RUnlock()

	// Ensure cleanup.
	defer func() {
		agent.mu.Lock()
		delete(agent.PendingCmds, cmd.CommandId)
		agent.mu.Unlock()
	}()

	// Wait for response with timeout.
	select {
	case result, ok := <-responseCh:
		if !ok {
			return nil, fmt.Errorf("agent %s disconnected while waiting for command result", agentID)
		}
		return result, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("command %s to agent %s timed out", cmd.CommandId, agentID)
	}
}

// IsOnline returns true if the agent is currently connected.
func (m *Manager) IsOnline(agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.agents[agentID]
	return ok
}

// HandleCommandResult routes a command result to the waiting caller.
func (m *Manager) HandleCommandResult(agentID string, result *backupv1.CommandResult) {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	agent.mu.Lock()
	ch, ok := agent.PendingCmds[result.CommandId]
	agent.mu.Unlock()
	if !ok {
		return
	}

	select {
	case ch <- result:
	default:
		// Result channel already has a value or is full; discard duplicate.
	}
}

// HeartbeatJobTransition represents a change in the agent's current job state.
type HeartbeatJobTransition int

const (
	// JobNoChange means no change in job state.
	JobNoChange HeartbeatJobTransition = iota
	// JobStarted means a new job started executing.
	JobStarted
	// JobProgress means the job is still running with updated progress.
	JobProgress
	// JobStopped means the agent was running a job but is now idle (job finished or aborted).
	JobStopped
)

// UpdateHeartbeat updates the last heartbeat time, status, and current job info.
// Returns the job transition type and the current job info (if any).
func (m *Manager) UpdateHeartbeat(agentID, status string, currentJob *CurrentJobInfo) HeartbeatJobTransition {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()
	if !ok {
		return JobNoChange
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.LastHeart = time.Now()
	agent.Status = status

	prevJob := agent.CurrentJob
	agent.CurrentJob = currentJob

	switch {
	case prevJob == nil && currentJob != nil:
		return JobStarted
	case prevJob != nil && currentJob == nil:
		return JobStopped
	case prevJob != nil && currentJob != nil:
		return JobProgress
	default:
		return JobNoChange
	}
}

// GetCurrentJob returns the current job info for an agent (if any).
func (m *Manager) GetCurrentJob(agentID string) *CurrentJobInfo {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return agent.CurrentJob
}

// Close unregisters all connected agents and releases resources.
func (m *Manager) Close() {
	m.once.Do(func() {
		close(m.done)

		m.mu.Lock()
		defer m.mu.Unlock()
		for id, agent := range m.agents {
			agent.mu.Lock()
			for cmdID, ch := range agent.PendingCmds {
				close(ch)
				delete(agent.PendingCmds, cmdID)
			}
			agent.mu.Unlock()
			close(agent.SendCh)
			delete(m.agents, id)
		}
	})
}
