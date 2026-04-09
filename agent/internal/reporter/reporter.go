package reporter

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tryy3/backup-orchestrator/agent/internal/database"
	"github.com/tryy3/backup-orchestrator/agent/internal/grpcclient"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// Reporter manages buffered job report delivery to the server.
// It buffers reports locally in SQLite when the server is unreachable
// and flushes them when connectivity is restored.
type Reporter struct {
	db       *database.DB
	grpc     *grpcclient.Client
	interval time.Duration
	flushCh  chan struct{}
	mu       sync.Mutex
}

// New creates a new Reporter.
func New(db *database.DB, grpcClient *grpcclient.Client, interval time.Duration) *Reporter {
	return &Reporter{
		db:       db,
		grpc:     grpcClient,
		interval: interval,
		flushCh:  make(chan struct{}, 1),
	}
}

// Run starts the periodic flush loop. It runs until the context is cancelled.
func (r *Reporter) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.flush(ctx)
		case <-r.flushCh:
			r.flush(ctx)
		}
	}
}

// BufferReport marshals a JobReport to JSON and inserts it into the local buffer.
func (r *Reporter) BufferReport(report *backupv1.JobReport) error {
	data, err := protojson.Marshal(report)
	if err != nil {
		return err
	}

	id := uuid.New().String()
	return r.db.InsertBufferedReport(id, string(data))
}

// FlushNow triggers an immediate flush of buffered reports.
func (r *Reporter) FlushNow() {
	select {
	case r.flushCh <- struct{}{}:
	default:
		// Flush already pending.
	}
}

// maxFlushAttempts is the maximum number of delivery attempts before a
// buffered report is discarded.
const maxFlushAttempts = 10

func (r *Reporter) flush(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	reports, err := r.db.ListPendingReports()
	if err != nil {
		slog.Error("error listing pending reports", "source", "reporter", "error", err)
		return
	}

	if len(reports) == 0 {
		return
	}

	slog.Info("flushing buffered reports", "source", "reporter", "count", len(reports))

	for _, br := range reports {
		// Drop reports that have exceeded max attempts.
		if br.Attempts >= maxFlushAttempts {
			slog.Warn("dropping report after max attempts", "source", "reporter", "report_id", br.ID, "attempts", br.Attempts, "last_error", br.LastError)
			if err := r.db.DeleteReport(br.ID); err != nil {
				slog.Error("error deleting expired report", "source", "reporter", "report_id", br.ID, "error", err)
			}
			continue
		}

		var report backupv1.JobReport
		if err := protojson.Unmarshal([]byte(br.Payload), &report); err != nil {
			slog.Error("error unmarshaling report", "source", "reporter", "report_id", br.ID, "error", err)
			r.db.IncrementAttempts(br.ID, err.Error())
			continue
		}

		if err := r.grpc.ReportJob(ctx, &report); err != nil {
			slog.Error("error sending report", "source", "reporter", "report_id", br.ID, "error", err)
			r.db.IncrementAttempts(br.ID, err.Error())
			continue
		}

		if err := r.db.DeleteReport(br.ID); err != nil {
			slog.Error("error deleting report", "source", "reporter", "report_id", br.ID, "error", err)
		} else {
			slog.Info("successfully sent report", "source", "reporter", "report_id", br.ID)
		}
	}
}
