package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestBufferHandler_CapturesEntries(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	logger := slog.New(h)

	logger.Info("hello", "source", "test", "key", "value")
	logger.Warn("warning msg")

	entries := h.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "hello" {
		t.Errorf("message: got %q, want %q", entries[0].Message, "hello")
	}
	if entries[0].Level != "info" {
		t.Errorf("level: got %q, want %q", entries[0].Level, "info")
	}
	if entries[0].Source != "test" {
		t.Errorf("source: got %q, want %q", entries[0].Source, "test")
	}
	if entries[1].Level != "warn" {
		t.Errorf("level: got %q, want %q", entries[1].Level, "warn")
	}
}

func TestBufferHandler_FiltersLevel(t *testing.T) {
	h := NewBufferHandler(slog.LevelWarn)
	logger := slog.New(h)

	logger.Info("should be skipped")
	logger.Warn("should appear")

	entries := h.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != "should appear" {
		t.Errorf("message: got %q", entries[0].Message)
	}
}

func TestBufferHandler_MaxEntries(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	logger := slog.New(h)

	for i := 0; i < maxEntries+100; i++ {
		logger.Info("msg")
	}

	entries := h.Entries()
	if len(entries) != maxEntries {
		t.Errorf("expected %d entries, got %d", maxEntries, len(entries))
	}
}

func TestBufferHandler_Attributes(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	logger := slog.New(h)

	logger.Info("test", "key1", "val1", "key2", "val2")

	entries := h.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	var attrs map[string]string
	if err := json.Unmarshal([]byte(entries[0].Attributes), &attrs); err != nil {
		t.Fatalf("unmarshal attributes: %v", err)
	}
	if attrs["key1"] != "val1" || attrs["key2"] != "val2" {
		t.Errorf("attributes: %v", attrs)
	}
}

func TestBufferHandler_WithGroup(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	logger := slog.New(h.WithGroup("grp"))

	logger.Info("test", "key", "val")

	entries := h.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	var attrs map[string]string
	if err := json.Unmarshal([]byte(entries[0].Attributes), &attrs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if attrs["grp.key"] != "val" {
		t.Errorf("expected grouped key 'grp.key', got attrs: %v", attrs)
	}
}

func TestBufferHandler_NestedGroups(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	logger := slog.New(h.WithGroup("a").WithGroup("b"))

	logger.Info("test", "key", "val")

	entries := h.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	var attrs map[string]string
	if err := json.Unmarshal([]byte(entries[0].Attributes), &attrs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if attrs["a.b.key"] != "val" {
		t.Errorf("expected key 'a.b.key', got attrs: %v", attrs)
	}
}

func TestBufferHandler_WithGroupEmptyName(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	h2 := h.WithGroup("")
	if h2 != h {
		t.Error("WithGroup(\"\") should return the same handler")
	}
}

func TestBufferHandler_WithAttrsShared(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)
	l1 := slog.New(h.WithAttrs([]slog.Attr{slog.String("source", "s1")}))
	l2 := slog.New(h.WithAttrs([]slog.Attr{slog.String("source", "s2")}))

	l1.Info("from l1")
	l2.Info("from l2")

	entries := h.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Source != "s1" {
		t.Errorf("entry 0 source: got %q, want s1", entries[0].Source)
	}
	if entries[1].Source != "s2" {
		t.Errorf("entry 1 source: got %q, want s2", entries[1].Source)
	}
}

func TestBufferHandler_PlainText(t *testing.T) {
	h := NewBufferHandler(slog.LevelInfo)

	r := slog.NewRecord(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), slog.LevelInfo, "hello world", 0)
	r.AddAttrs(slog.String("source", "test"))
	h.Handle(context.Background(), r)

	text := h.PlainText()
	if !strings.Contains(text, "hello world") {
		t.Errorf("PlainText missing message: %s", text)
	}
	if !strings.Contains(text, "[test]") {
		t.Errorf("PlainText missing source: %s", text)
	}
	if !strings.Contains(text, "[INFO ]") {
		t.Errorf("PlainText missing level: %s", text)
	}
}

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
	}
	for _, tt := range tests {
		got := levelToString(tt.level)
		if got != tt.want {
			t.Errorf("levelToString(%v): got %q, want %q", tt.level, got, tt.want)
		}
	}
}
