package agentmgr

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

func TestManager_RegisterUnregister(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh)
	assert.True(t, mgr.IsOnline("agent-1"))

	mgr.Unregister("agent-1")
	assert.False(t, mgr.IsOnline("agent-1"))
}

func TestManager_UnregisterUnknown(t *testing.T) {
	t.Parallel()

	mgr := New()
	mgr.Unregister("nonexistent") // Should not panic.
}

func TestManager_UnregisterClosesChannels(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh)
	mgr.Unregister("agent-1")

	// Send channel should be closed.
	_, ok := <-sendCh
	assert.False(t, ok, "send channel should be closed after unregister")
}

func TestManager_Send(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	msg := &backupv1.ServerMessage{}
	err := mgr.Send("agent-1", msg)
	require.NoError(t, err)

	select {
	case received := <-sendCh:
		assert.Equal(t, msg, received)
	case <-time.After(time.Second):
		t.Fatal("message not received")
	}
}

func TestManager_SendNotConnected(t *testing.T) {
	t.Parallel()

	mgr := New()
	err := mgr.Send("unknown", &backupv1.ServerMessage{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestManager_SendBufferFull(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 1)

	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	// Fill the buffer.
	err := mgr.Send("agent-1", &backupv1.ServerMessage{})
	require.NoError(t, err)

	// Next send should fail (buffer full).
	err = mgr.Send("agent-1", &backupv1.ServerMessage{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "buffer full")
}

func TestManager_IsOnline(t *testing.T) {
	t.Parallel()

	mgr := New()

	assert.False(t, mgr.IsOnline("agent-1"))

	sendCh := make(chan *backupv1.ServerMessage, 32)
	mgr.Register("agent-1", sendCh)
	assert.True(t, mgr.IsOnline("agent-1"))

	mgr.Unregister("agent-1")
	assert.False(t, mgr.IsOnline("agent-1"))
}

func TestManager_SendCommand(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	// Simulate the agent responding in a goroutine.
	go func() {
		msg := <-sendCh
		cmd := msg.GetCommand()
		require.NotNil(t, cmd)
		mgr.HandleCommandResult("agent-1", &backupv1.CommandResult{
			CommandId: cmd.CommandId,
			Success:   true,
		})
	}()

	cmd := &backupv1.Command{
		Action: &backupv1.Command_TriggerBackup{
			TriggerBackup: &backupv1.TriggerBackup{PlanId: "plan-1"},
		},
	}
	result, err := mgr.SendCommand("agent-1", cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestManager_SendCommandAssignsID(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	// Respond immediately.
	go func() {
		msg := <-sendCh
		cmd := msg.GetCommand()
		mgr.HandleCommandResult("agent-1", &backupv1.CommandResult{
			CommandId: cmd.CommandId,
			Success:   true,
		})
	}()

	cmd := &backupv1.Command{} // No CommandId set.
	result, err := mgr.SendCommand("agent-1", cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotEmpty(t, cmd.CommandId, "SendCommand should assign a CommandId")
}

func TestManager_SendCommandNotConnected(t *testing.T) {
	t.Parallel()

	mgr := New()
	_, err := mgr.SendCommand("unknown", &backupv1.Command{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestManager_SendCommandTimeout(t *testing.T) {
	t.Parallel()

	// This test verifies the 30-second timeout. We can't actually wait 30s,
	// so we just verify the mechanism works when the agent disconnects.
	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh)

	// Start command in goroutine.
	done := make(chan error, 1)
	go func() {
		_, err := mgr.SendCommand("agent-1", &backupv1.Command{})
		done <- err
	}()

	// Unregister agent — this closes pending command channels.
	time.Sleep(10 * time.Millisecond) // Let SendCommand register the pending command.
	mgr.Unregister("agent-1")

	select {
	case err := <-done:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "disconnected")
	case <-time.After(5 * time.Second):
		t.Fatal("SendCommand did not return after agent disconnect")
	}
}

func TestManager_HandleCommandResult_UnknownAgent(t *testing.T) {
	t.Parallel()

	mgr := New()
	// Should not panic.
	mgr.HandleCommandResult("unknown", &backupv1.CommandResult{CommandId: "cmd-1"})
}

func TestManager_HandleCommandResult_UnknownCommand(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)
	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	// Should not panic when no pending command exists.
	mgr.HandleCommandResult("agent-1", &backupv1.CommandResult{CommandId: "nonexistent"})
}

func TestManager_UpdateHeartbeat(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)
	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	tests := []struct {
		name       string
		status     string
		job        *CurrentJobInfo
		prevJob    *CurrentJobInfo
		wantResult HeartbeatJobTransition
	}{
		{
			name:       "idle to idle",
			status:     "idle",
			job:        nil,
			prevJob:    nil,
			wantResult: JobNoChange,
		},
		{
			name:       "idle to running",
			status:     "running",
			job:        &CurrentJobInfo{PlanName: "daily"},
			prevJob:    nil,
			wantResult: JobStarted,
		},
		{
			name:       "running progress",
			status:     "running",
			job:        &CurrentJobInfo{PlanName: "daily", ProgressPercent: 50},
			prevJob:    &CurrentJobInfo{PlanName: "daily", ProgressPercent: 25},
			wantResult: JobProgress,
		},
		{
			name:       "running to idle",
			status:     "idle",
			job:        nil,
			prevJob:    &CurrentJobInfo{PlanName: "daily"},
			wantResult: JobStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up previous job state.
			mgr.mu.RLock()
			agent := mgr.agents["agent-1"]
			mgr.mu.RUnlock()
			agent.mu.Lock()
			agent.CurrentJob = tt.prevJob
			agent.mu.Unlock()

			result := mgr.UpdateHeartbeat("agent-1", tt.status, tt.job)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestManager_UpdateHeartbeat_UnknownAgent(t *testing.T) {
	t.Parallel()

	mgr := New()
	result := mgr.UpdateHeartbeat("unknown", "idle", nil)
	assert.Equal(t, JobNoChange, result)
}

func TestManager_GetCurrentJob(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)
	mgr.Register("agent-1", sendCh)
	defer mgr.Unregister("agent-1")

	assert.Nil(t, mgr.GetCurrentJob("agent-1"))

	job := &CurrentJobInfo{PlanName: "daily", ProgressPercent: 42}
	mgr.UpdateHeartbeat("agent-1", "running", job)

	got := mgr.GetCurrentJob("agent-1")
	require.NotNil(t, got)
	assert.Equal(t, "daily", got.PlanName)
	assert.InDelta(t, float32(42), got.ProgressPercent, 0.01)
}

func TestManager_GetCurrentJob_UnknownAgent(t *testing.T) {
	t.Parallel()

	mgr := New()
	assert.Nil(t, mgr.GetCurrentJob("unknown"))
}

func TestManager_Close(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh1 := make(chan *backupv1.ServerMessage, 32)
	sendCh2 := make(chan *backupv1.ServerMessage, 32)

	mgr.Register("agent-1", sendCh1)
	mgr.Register("agent-2", sendCh2)

	mgr.Close()

	// All agents should be gone.
	assert.False(t, mgr.IsOnline("agent-1"))
	assert.False(t, mgr.IsOnline("agent-2"))

	// Send channels should be closed.
	_, ok1 := <-sendCh1
	assert.False(t, ok1)
	_, ok2 := <-sendCh2
	assert.False(t, ok2)
}

func TestManager_CloseIdempotent(t *testing.T) {
	t.Parallel()

	mgr := New()
	sendCh := make(chan *backupv1.ServerMessage, 32)
	mgr.Register("agent-1", sendCh)

	mgr.Close()
	mgr.Close() // Should not panic.
}

func TestManager_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	mgr := New()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "agent-" + string(rune('a'+n))
			ch := make(chan *backupv1.ServerMessage, 32)
			mgr.Register(id, ch)
			mgr.IsOnline(id)
			mgr.Send(id, &backupv1.ServerMessage{})
			mgr.UpdateHeartbeat(id, "idle", nil)
			mgr.GetCurrentJob(id)
			mgr.Unregister(id)
		}(i)
	}
	wg.Wait()
}
