// Package outbox implements an in-memory-first delivery queue for messages
// the agent must hand off to the server (job reports today, job events
// later). Items are delivered immediately when the server is reachable and
// only spill to SQLite when the in-memory channel is full or delivery fails.
//
// Capacity policy (two-tier, both tiers configurable):
//
//  1. In-memory bounded channel sized at Config.MemoryMax (default 2000).
//     Submit() never blocks: if the channel is full, the item is written
//     to the SQLite spill table instead.
//
//  2. SQLite spill table bounded at Config.SpillMaxRows (default 20000).
//     If the spill is full when Submit() needs it, the oldest rows are
//     evicted to make room (drop-oldest).
//
// The worker drains the channel preferentially, then pages through the
// spill table 50 rows at a time so the resident set is always bounded.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tryy3/backup-orchestrator/agent/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"google.golang.org/protobuf/proto"
)

// Kinds.
const (
	KindJobReport = "job_report"
	KindJobEvent  = "job_event" // Reserved for Phase 4.
)

// pageSize is how many spilled items the worker pulls per page.
const pageSize = 50

// Sender is the outbound transport interface. grpcclient.Client satisfies it.
type Sender interface {
	ReportJob(ctx context.Context, report *backupv1.JobReport) error
}

// Config tunes the outbox. Zero values fall back to package defaults.
type Config struct {
	MemoryMax       int           // in-memory channel capacity (default 2000)
	SpillMaxRows    int           // SQLite spill row cap (default 20000)
	SpillRetention  time.Duration // age cutoff for prune (default 7d)
	FlushInterval   time.Duration // periodic flush tick (default 60s)
	DeliveryTimeout time.Duration // per-RPC timeout (default 10s)
	MaxAttempts     int           // drop after N failures (default 10)
	BackoffInitial  time.Duration // first failure delay (default 1s)
	BackoffMax      time.Duration // capped failure delay (default 30s)
	PruneInterval   time.Duration // background prune cadence (default 24h)
}

func (c *Config) applyDefaults() {
	if c.MemoryMax <= 0 {
		c.MemoryMax = 2000
	}
	if c.SpillMaxRows <= 0 {
		c.SpillMaxRows = 20000
	}
	if c.SpillRetention <= 0 {
		c.SpillRetention = 7 * 24 * time.Hour
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 60 * time.Second
	}
	if c.DeliveryTimeout <= 0 {
		c.DeliveryTimeout = 10 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.BackoffInitial <= 0 {
		c.BackoffInitial = 1 * time.Second
	}
	if c.BackoffMax <= 0 {
		c.BackoffMax = 30 * time.Second
	}
	if c.PruneInterval <= 0 {
		c.PruneInterval = 24 * time.Hour
	}
}

// Outbox owns the in-memory queue + spill table and runs a worker that
// delivers items in order.
type Outbox struct {
	cfg    atomic.Pointer[Config]
	db     *database.DB
	sender Sender
	logger *slog.Logger

	mem           chan database.SpillItem
	flushCh       chan struct{}
	reloadFlushCh chan struct{} // resets the Run loop's flush ticker
	reloadPruneCh chan struct{} // resets the prune loop's ticker
}

// New constructs an Outbox. Call Run in a goroutine to start delivering.
func New(db *database.DB, sender Sender, cfg Config, logger *slog.Logger) *Outbox {
	cfg.applyDefaults()
	if logger == nil {
		logger = slog.Default()
	}
	o := &Outbox{
		db:            db,
		sender:        sender,
		logger:        logger.With("source", "outbox"),
		mem:           make(chan database.SpillItem, cfg.MemoryMax),
		flushCh:       make(chan struct{}, 1),
		reloadFlushCh: make(chan struct{}, 1),
		reloadPruneCh: make(chan struct{}, 1),
	}
	o.cfg.Store(&cfg)
	return o
}

// config returns a snapshot of the current configuration. Callers should
// capture the result once per operation rather than re-loading mid-flight
// so values stay coherent.
func (o *Outbox) config() *Config {
	return o.cfg.Load()
}

// UpdateConfig hot-swaps the outbox tunables. The in-memory channel
// capacity (MemoryMax) is preserved from the original Config because Go
// channels cannot be resized at runtime — pass the same value or 0.
// Zero-valued fields fall back to the package defaults via applyDefaults.
func (o *Outbox) UpdateConfig(next Config) {
	current := o.cfg.Load()
	next.MemoryMax = current.MemoryMax // bootstrap value, never changes
	next.applyDefaults()
	o.cfg.Store(&next)
	select {
	case o.reloadFlushCh <- struct{}{}:
	default:
	}
	select {
	case o.reloadPruneCh <- struct{}{}:
	default:
	}
}

// SubmitReport enqueues a JobReport for delivery. It never blocks.
//
// If the in-memory channel is full, the report is spilled to SQLite. If the
// spill table is also at its row cap, the oldest rows are evicted first.
func (o *Outbox) SubmitReport(ctx context.Context, report *backupv1.JobReport) error {
	payload, err := proto.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	item := database.SpillItem{
		ID:      uuid.NewString(),
		Kind:    KindJobReport,
		Payload: payload,
	}
	return o.submit(ctx, item)
}

// submit places an item into the in-memory channel or spills it to SQLite.
func (o *Outbox) submit(ctx context.Context, item database.SpillItem) error {
	cfg := o.config()
	select {
	case o.mem <- item:
		return nil
	default:
		// Memory tier full: spill, after evicting the oldest rows if we are
		// at the configured cap.
		count, err := o.db.SpillCount(ctx)
		if err != nil {
			return fmt.Errorf("spill count: %w", err)
		}
		if count >= cfg.SpillMaxRows {
			over := count - cfg.SpillMaxRows + 1
			deleted, derr := o.db.SpillDeleteOldest(ctx, over)
			if derr != nil {
				return fmt.Errorf("spill evict: %w", derr)
			}
			o.logger.Warn("spill full, dropped oldest rows",
				"dropped", deleted, "cap", cfg.SpillMaxRows)
		}
		if err := o.db.SpillEnqueue(ctx, item); err != nil {
			return fmt.Errorf("spill enqueue: %w", err)
		}
		return nil
	}
}

// FlushNow nudges the worker to drain immediately. Safe to call from any
// goroutine; coalesced when one is already pending.
func (o *Outbox) FlushNow() {
	select {
	case o.flushCh <- struct{}{}:
	default:
	}
}

// Run drains the outbox until ctx is cancelled. It always runs the prune
// loop as a child goroutine.
func (o *Outbox) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.runPrune(ctx)
	}()

	ticker := time.NewTicker(o.config().FlushInterval)
	defer ticker.Stop()

	for {
		// Drain everything we can in this tick.
		o.drainOnce(ctx)

		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
		case <-o.flushCh:
		case <-o.reloadFlushCh:
			ticker.Reset(o.config().FlushInterval)
		case item := <-o.mem:
			// New work arrived; deliver it then loop back to drain.
			o.deliver(ctx, item, false)
		}
	}
}

// drainOnce delivers everything currently buffered: in-memory first, then
// the SQLite spill in pages.
func (o *Outbox) drainOnce(ctx context.Context) {
	// 1. Drain the in-memory channel non-blockingly.
	for {
		select {
		case item := <-o.mem:
			o.deliver(ctx, item, false)
		default:
			goto spill
		}
		if ctx.Err() != nil {
			return
		}
	}
spill:
	// 2. Drain the spill table in pages so the resident set is bounded.
	var (
		afterCreated string
		afterID      string
	)
	for {
		if ctx.Err() != nil {
			return
		}
		items, err := o.db.SpillPage(ctx, pageSize, afterCreated, afterID)
		if err != nil {
			o.logger.Error("spill page", "error", err)
			return
		}
		if len(items) == 0 {
			return
		}
		for _, it := range items {
			if ctx.Err() != nil {
				return
			}
			o.deliver(ctx, it, true)
			afterCreated = it.CreatedAt
			afterID = it.ID
		}
	}
}

// deliver sends one item. On failure it spills (if from memory) or
// increments attempts (if already spilled), then backs off briefly. Items
// past MaxAttempts are dropped with a warning.
func (o *Outbox) deliver(ctx context.Context, item database.SpillItem, fromSpill bool) {
	cfg := o.config()
	if item.Attempts >= cfg.MaxAttempts {
		o.logger.Warn("dropping item after max attempts",
			"id", item.ID, "kind", item.Kind, "attempts", item.Attempts,
			"last_error", item.LastError)
		if fromSpill {
			if err := o.db.SpillDelete(ctx, item.ID); err != nil {
				o.logger.Error("spill delete after drop", "id", item.ID, "error", err)
			}
		}
		return
	}

	rpcCtx, cancel := context.WithTimeout(ctx, cfg.DeliveryTimeout)
	err := o.send(rpcCtx, item)
	cancel()

	if err == nil {
		if fromSpill {
			if delErr := o.db.SpillDelete(ctx, item.ID); delErr != nil {
				o.logger.Error("spill delete after send", "id", item.ID, "error", delErr)
			}
		}
		return
	}

	if errors.Is(err, context.Canceled) {
		// Shutdown in progress: persist the item so we retry next start.
		if !fromSpill {
			spillCtx, spillCancel := context.WithTimeout(context.Background(), cfg.DeliveryTimeout)
			sErr := o.db.SpillEnqueue(spillCtx, item)
			spillCancel()
			if sErr != nil {
				o.logger.Error("spill on shutdown", "id", item.ID, "error", sErr)
			}
		}
		return
	}

	o.logger.Warn("delivery failed", "id", item.ID, "kind", item.Kind, "error", err)
	if !fromSpill {
		// First failure for an in-memory item: spill it so the retry
		// counter is durable across restarts.
		item.Attempts++
		item.LastError = err.Error()
		if sErr := o.db.SpillEnqueue(ctx, item); sErr != nil {
			o.logger.Error("spill after failure", "id", item.ID, "error", sErr)
		}
	} else {
		if iErr := o.db.SpillIncrementAttempts(ctx, item.ID, err.Error()); iErr != nil {
			o.logger.Error("increment attempts", "id", item.ID, "error", iErr)
		}
	}
	o.backoff(ctx, item.Attempts)
}

// send dispatches one item to the right Sender method by kind.
func (o *Outbox) send(ctx context.Context, item database.SpillItem) error {
	switch item.Kind {
	case KindJobReport:
		var report backupv1.JobReport
		if err := proto.Unmarshal(item.Payload, &report); err != nil {
			return fmt.Errorf("unmarshal report: %w", err)
		}
		return o.sender.ReportJob(ctx, &report)
	default:
		// Unknown kinds are dropped after one failed attempt; treat as a
		// permanent error so MaxAttempts cleans them up.
		return fmt.Errorf("unsupported outbox kind: %q", item.Kind)
	}
}

// backoff sleeps with jitter, scaled by attempt count.
func (o *Outbox) backoff(ctx context.Context, attempts int) {
	cfg := o.config()
	d := cfg.BackoffInitial << min(attempts, 5) //nolint:gosec // small int
	if d > cfg.BackoffMax {
		d = cfg.BackoffMax
	}
	jitter := time.Duration(rand.Int64N(int64(d) / 4)) //nolint:gosec // not security-sensitive
	select {
	case <-ctx.Done():
	case <-time.After(d + jitter):
	}
}

// runPrune trims the spill table by age and row count, then checkpoints
// the WAL to reclaim disk.
func (o *Outbox) runPrune(ctx context.Context) {
	o.pruneOnce(ctx) // run on startup
	t := time.NewTicker(o.config().PruneInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.pruneOnce(ctx)
		case <-o.reloadPruneCh:
			t.Reset(o.config().PruneInterval)
		}
	}
}

func (o *Outbox) pruneOnce(ctx context.Context) {
	cfg := o.config()
	cutoff := time.Now().Add(-cfg.SpillRetention).UTC().Format("2006-01-02 15:04:05")
	if n, err := o.db.SpillPruneByAge(ctx, cutoff); err != nil {
		o.logger.Error("prune by age", "error", err)
	} else if n > 0 {
		o.logger.Info("pruned spill rows by age", "count", n, "cutoff", cutoff)
	}
	if n, err := o.db.SpillPruneByCount(ctx, cfg.SpillMaxRows); err != nil {
		o.logger.Error("prune by count", "error", err)
	} else if n > 0 {
		o.logger.Info("pruned spill rows by count", "count", n, "cap", cfg.SpillMaxRows)
	}
	if err := o.db.SpillCheckpoint(ctx); err != nil {
		o.logger.Debug("wal checkpoint", "error", err)
	}
}
