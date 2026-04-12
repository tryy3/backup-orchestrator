package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

const maxEntries = 1000

// entryBuffer is the shared storage for log entries across WithAttrs copies.
type entryBuffer struct {
	mu      sync.Mutex
	entries []*backupv1.LogEntry
	notify  chan<- *backupv1.LogEntry // optional; non-nil when live-log streaming is enabled
}

// BufferHandler is a slog.Handler that captures log entries into a buffer
// for later retrieval (e.g., attaching to a job report).
type BufferHandler struct {
	buf    *entryBuffer
	level  slog.Level
	attrs  []slog.Attr
	groups []string // active group names for key prefixing
}

// NewBufferHandler creates a BufferHandler that captures entries at or above the given level.
func NewBufferHandler(level slog.Level) *BufferHandler {
	return &BufferHandler{
		buf:   &entryBuffer{},
		level: level,
	}
}

// NewBufferHandlerWithNotify creates a BufferHandler that also sends each entry
// to the provided channel (non-blocking) for live-log streaming.
func NewBufferHandlerWithNotify(level slog.Level, ch chan<- *backupv1.LogEntry) *BufferHandler {
	return &BufferHandler{
		buf: &entryBuffer{
			notify: ch,
		},
		level: level,
	}
}

func (h *BufferHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *BufferHandler) Handle(_ context.Context, r slog.Record) error {
	h.buf.mu.Lock()
	defer h.buf.mu.Unlock()

	if len(h.buf.entries) >= maxEntries {
		return nil
	}

	entry := &backupv1.LogEntry{
		Timestamp: r.Time.Format(time.RFC3339),
		Level:     levelToString(r.Level),
		Message:   r.Message,
	}

	// Collect attributes: extract "source" separately, rest go into JSON.
	attrs := make(map[string]string)
	prefix := strings.Join(h.groups, ".")
	for _, a := range h.attrs {
		key := a.Key
		if prefix != "" {
			key = prefix + "." + key
		}
		if key == "source" {
			entry.Source = a.Value.String()
		} else {
			attrs[key] = a.Value.String()
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if prefix != "" {
			key = prefix + "." + key
		}
		if key == "source" {
			entry.Source = a.Value.String()
		} else {
			attrs[key] = a.Value.String()
		}
		return true
	})

	if len(attrs) > 0 {
		if data, err := json.Marshal(attrs); err == nil {
			entry.Attributes = string(data)
		}
	}

	h.buf.entries = append(h.buf.entries, entry)

	// Non-blocking send to live-log channel (if enabled).
	if h.buf.notify != nil {
		select {
		case h.buf.notify <- entry:
		default:
		}
	}

	return nil
}

func (h *BufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BufferHandler{
		buf:    h.buf, // share the same buffer
		level:  h.level,
		attrs:  append(cloneAttrs(h.attrs), attrs...),
		groups: h.groups,
	}
}

func (h *BufferHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &BufferHandler{
		buf:    h.buf,
		level:  h.level,
		attrs:  cloneAttrs(h.attrs),
		groups: append(cloneStrings(h.groups), name),
	}
}

// Entries returns the captured log entries.
func (h *BufferHandler) Entries() []*backupv1.LogEntry {
	h.buf.mu.Lock()
	defer h.buf.mu.Unlock()
	out := make([]*backupv1.LogEntry, len(h.buf.entries))
	copy(out, h.buf.entries)
	return out
}

// PlainText renders all entries as a human-readable string for the log_tail field.
func (h *BufferHandler) PlainText() string {
	h.buf.mu.Lock()
	defer h.buf.mu.Unlock()

	var sb strings.Builder
	for _, e := range h.buf.entries {
		fmt.Fprintf(&sb, "%s [%-5s] [%s] %s", e.Timestamp, strings.ToUpper(e.Level), e.Source, e.Message)
		if e.Attributes != "" {
			fmt.Fprintf(&sb, " %s", e.Attributes)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func levelToString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

func cloneAttrs(attrs []slog.Attr) []slog.Attr {
	if attrs == nil {
		return nil
	}
	out := make([]slog.Attr, len(attrs))
	copy(out, attrs)
	return out
}

func cloneStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
