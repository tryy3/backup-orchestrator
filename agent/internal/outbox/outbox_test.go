package outbox

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"google.golang.org/protobuf/proto"
)

// fakeSender records calls and lets tests script return values.
type fakeSender struct {
	mu       sync.Mutex
	calls    []string
	errFn    func(*backupv1.JobReport) error
	delivery int32
}

func (f *fakeSender) ReportJob(_ context.Context, r *backupv1.JobReport) error {
	f.mu.Lock()
	f.calls = append(f.calls, r.JobId)
	f.mu.Unlock()
	atomic.AddInt32(&f.delivery, 1)
	if f.errFn != nil {
		return f.errFn(r)
	}
	return nil
}

func (f *fakeSender) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func newTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestOutbox(t *testing.T, db *database.DB, sender Sender, override func(*Config)) *Outbox {
	t.Helper()
	cfg := Config{
		MemoryMax:       8,
		SpillMaxRows:    100,
		FlushInterval:   100 * time.Millisecond,
		DeliveryTimeout: 200 * time.Millisecond,
		MaxAttempts:     3,
		BackoffInitial:  1 * time.Millisecond,
		BackoffMax:      5 * time.Millisecond,
		PruneInterval:   1 * time.Hour,
	}
	if override != nil {
		override(&cfg)
	}
	return New(db, sender, cfg, nil)
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", d)
}

func TestSubmit_DeliveredFromMemory(t *testing.T) {
	db := newTestDB(t)
	sender := &fakeSender{}
	ob := newTestOutbox(t, db, sender, nil)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go ob.Run(ctx)

	for _, id := range []string{"j1", "j2", "j3"} {
		if err := ob.SubmitReport(ctx, &backupv1.JobReport{JobId: id}); err != nil {
			t.Fatalf("submit %s: %v", id, err)
		}
	}

	waitFor(t, time.Second, func() bool { return len(sender.seen()) == 3 })

	count, err := db.SpillCount(ctx)
	if err != nil {
		t.Fatalf("SpillCount: %v", err)
	}
	if count != 0 {
		t.Errorf("spill should be empty after success, got %d", count)
	}
}

func TestSubmit_SpillsWhenMemoryFull(t *testing.T) {
	db := newTestDB(t)
	// Sender that always fails so the outbox can't drain and the channel
	// fills. We don't run the worker — pure submit-path test.
	ob := newTestOutbox(t, db, &fakeSender{}, func(c *Config) { c.MemoryMax = 2 })

	ctx := t.Context()
	for i := 0; i < 5; i++ {
		if err := ob.SubmitReport(ctx, &backupv1.JobReport{JobId: "j"}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	count, err := db.SpillCount(ctx)
	if err != nil {
		t.Fatalf("SpillCount: %v", err)
	}
	if count != 3 { // 5 submitted, 2 fit in memory
		t.Errorf("spill count: got %d, want 3", count)
	}
}

func TestSubmit_EvictsOldestWhenSpillFull(t *testing.T) {
	db := newTestDB(t)
	ob := newTestOutbox(t, db, &fakeSender{}, func(c *Config) {
		c.MemoryMax = 1
		c.SpillMaxRows = 3
	})

	ctx := t.Context()
	// Fill memory (1) + spill (3), then submit 2 more — should evict oldest.
	for i := 0; i < 6; i++ {
		if err := ob.SubmitReport(ctx, &backupv1.JobReport{JobId: "j"}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	count, err := db.SpillCount(ctx)
	if err != nil {
		t.Fatalf("SpillCount: %v", err)
	}
	if count != 3 {
		t.Errorf("spill capped at 3, got %d", count)
	}
}

func TestDeliver_RetriesAndPersistsOnFailure(t *testing.T) {
	db := newTestDB(t)
	var failures atomic.Int32
	sender := &fakeSender{errFn: func(_ *backupv1.JobReport) error {
		failures.Add(1)
		return errors.New("server down")
	}}
	ob := newTestOutbox(t, db, sender, nil)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go ob.Run(ctx)

	if err := ob.SubmitReport(ctx, &backupv1.JobReport{JobId: "j1"}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Wait for at least one failure, then verify the item was spilled
	// (memory submit → failure → spill enqueue with attempts++).
	waitFor(t, time.Second, func() bool { return failures.Load() >= 1 })
	waitFor(t, time.Second, func() bool {
		c, _ := db.SpillCount(ctx)
		return c == 1
	})

	items, err := db.SpillPage(ctx, 10, "", "")
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if items[0].Attempts < 1 {
		t.Errorf("attempts: got %d, want >=1", items[0].Attempts)
	}
}

func TestDeliver_DropsAfterMaxAttempts(t *testing.T) {
	db := newTestDB(t)
	sender := &fakeSender{errFn: func(_ *backupv1.JobReport) error {
		return errors.New("nope")
	}}
	ob := newTestOutbox(t, db, sender, func(c *Config) {
		c.MaxAttempts = 2
		c.BackoffInitial = time.Microsecond
		c.BackoffMax = time.Microsecond
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go ob.Run(ctx)

	if err := ob.SubmitReport(ctx, &backupv1.JobReport{JobId: "j1"}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// After 2 failed attempts the item is dropped.
	waitFor(t, 3*time.Second, func() bool {
		c, _ := db.SpillCount(ctx)
		return c == 0 && atomic.LoadInt32(&sender.delivery) >= 2
	})
}

func TestDrain_ProcessesSpillInPages(t *testing.T) {
	db := newTestDB(t)
	sender := &fakeSender{}
	// Cap above n so startup prune leaves the seeded rows alone.
	ob := newTestOutbox(t, db, sender, func(c *Config) { c.SpillMaxRows = 1000 })

	ctx := t.Context()
	// Pre-load the spill table directly with 120 items > pageSize (50).
	for i := 0; i < 120; i++ {
		if err := db.SpillEnqueue(ctx, database.SpillItem{
			ID:      "id-" + itoa(i),
			Kind:    KindJobReport,
			Payload: marshalReport(t, &backupv1.JobReport{JobId: "j" + itoa(i)}),
		}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go ob.Run(runCtx)

	waitFor(t, 5*time.Second, func() bool {
		c, _ := db.SpillCount(ctx)
		return c == 0
	})
	if got := atomic.LoadInt32(&sender.delivery); got != 120 {
		t.Errorf("delivered: got %d, want 120", got)
	}
}

func TestDrain_BoundedHeap(t *testing.T) {
	if testing.Short() {
		t.Skip("heap measurement skipped in short mode")
	}

	db := newTestDB(t)
	sender := &fakeSender{}
	// SpillMaxRows must exceed n so the startup prune doesn't drop rows.
	ob := newTestOutbox(t, db, sender, func(c *Config) { c.SpillMaxRows = 10_000 })

	ctx := t.Context()
	const n = 500
	const payloadSize = 4 << 10 // 4 KiB per row
	payload := make([]byte, payloadSize)
	for i := 0; i < n; i++ {
		_ = db.SpillEnqueue(ctx, database.SpillItem{
			ID:      "id-" + itoa(i),
			Kind:    KindJobReport,
			Payload: marshalReportWith(t, "j"+itoa(i), payload),
		})
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var peak atomic.Uint64
	done := make(chan struct{})
	go func() {
		var m runtime.MemStats
		for {
			select {
			case <-done:
				return
			default:
				runtime.ReadMemStats(&m)
				for {
					cur := peak.Load()
					if m.HeapInuse <= cur || peak.CompareAndSwap(cur, m.HeapInuse) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	go ob.Run(runCtx)
	waitFor(t, 10*time.Second, func() bool {
		c, _ := db.SpillCount(ctx)
		return c == 0
	})
	close(done)

	if int32(n) != atomic.LoadInt32(&sender.delivery) {
		t.Errorf("delivered: got %d, want %d", sender.delivery, n)
	}
	t.Logf("peak heap=%d total payload=%d", peak.Load(), n*payloadSize)
}

func TestPrune_ByAgeAndCount(t *testing.T) {
	db := newTestDB(t)
	ob := newTestOutbox(t, db, &fakeSender{}, func(c *Config) {
		c.SpillMaxRows = 2
		c.SpillRetention = 1 * time.Hour
	})

	ctx := t.Context()
	for i := 0; i < 5; i++ {
		if err := db.SpillEnqueue(ctx, database.SpillItem{
			ID:      "id-" + itoa(i),
			Kind:    KindJobReport,
			Payload: marshalReport(t, &backupv1.JobReport{JobId: "j"}),
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	ob.pruneOnce(ctx)

	c, err := db.SpillCount(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if c != 2 {
		t.Errorf("after prune count: got %d, want 2", c)
	}
}

// helpers

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

func marshalReport(t *testing.T, r *backupv1.JobReport) []byte {
	t.Helper()
	data, err := proto.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func marshalReportWith(t *testing.T, jobID string, blob []byte) []byte {
	t.Helper()
	r := &backupv1.JobReport{JobId: jobID, LogEntries: []string{string(blob)}}
	return marshalReport(t, r)
}
