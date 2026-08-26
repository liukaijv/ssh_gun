package applog

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotatingWriterRotatesBeforeWriteExceedsLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_gun.log")
	writer, err := newRotatingWriter(path, 5, 7)
	if err != nil {
		t.Fatalf("new rotating writer: %v", err)
	}

	if _, err := writer.Write([]byte("12345")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := writer.Write([]byte("6")); err != nil {
		t.Fatalf("rotating write: %v", err)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log: %v", err)
	}
	if got, want := string(current), "6"; got != want {
		t.Fatalf("current log = %q, want %q", got, want)
	}

	rotated, err := filepath.Glob(filepath.Join(dir, "ssh_gun-*.log"))
	if err != nil {
		t.Fatalf("glob rotated logs: %v", err)
	}
	if len(rotated) != 1 {
		t.Fatalf("rotated logs = %v, want exactly one", rotated)
	}
	archived, err := os.ReadFile(rotated[0])
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if got, want := string(archived), "12345"; got != want {
		t.Fatalf("rotated log = %q, want %q", got, want)
	}
}

func TestRotatingWriterCleansUpExpiredRotatedLogs(t *testing.T) {
	dir := t.TempDir()
	expired := filepath.Join(dir, "ssh_gun-20200101-000000.log")
	recent := filepath.Join(dir, "ssh_gun-20200102-000000.log")
	unrelated := filepath.Join(dir, "other-20200101-000000.log")
	for _, path := range []string{expired, recent, unrelated} {
		if err := os.WriteFile(path, []byte("log"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(expired, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour)); err != nil {
		t.Fatalf("age expired log: %v", err)
	}
	if err := os.Chtimes(recent, now.Add(-6*24*time.Hour), now.Add(-6*24*time.Hour)); err != nil {
		t.Fatalf("age recent log: %v", err)
	}
	if err := os.Chtimes(unrelated, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour)); err != nil {
		t.Fatalf("age unrelated log: %v", err)
	}

	if _, err := newRotatingWriter(filepath.Join(dir, "ssh_gun.log"), 100, 7); err != nil {
		t.Fatalf("new rotating writer: %v", err)
	}

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired rotated log still exists, stat error = %v", err)
	}
	for _, path := range []string{recent, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}
}

func TestRingBufferKeepsLastNLines(t *testing.T) {
	ring := NewRingBuffer(3)
	for _, line := range []string{"one", "two", "three", "four"} {
		ring.Append(line)
	}

	got := ring.Snapshot()
	want := []string{"two", "three", "four"}
	if len(got) != len(want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("snapshot = %v, want %v", got, want)
		}
	}

	got[0] = "mutated"
	if fresh := ring.Snapshot(); fresh[0] != "two" {
		t.Fatalf("Snapshot returned aliased storage: %v", fresh)
	}
}

func TestHandlerWritesJSONToFileSinkAndLineToRing(t *testing.T) {
	var file bytes.Buffer
	ring := NewRingBuffer(10)
	logger := slog.New(newHandler(&file, ring, slog.LevelDebug))

	logger.Info("connected", "host", "example.test")

	if got := file.String(); !strings.Contains(got, `"msg":"connected"`) ||
		!strings.Contains(got, `"host":"example.test"`) {
		t.Fatalf("file output = %q, want JSON message and attribute", got)
	}
	lines := ring.Snapshot()
	if len(lines) != 1 {
		t.Fatalf("ring lines = %v, want one", lines)
	}
	if !strings.Contains(lines[0], `"msg":"connected"`) ||
		!strings.Contains(lines[0], `"host":"example.test"`) {
		t.Fatalf("ring line = %q, want JSON message and attribute", lines[0])
	}
}

func TestHandlerFiltersDebugAtInfoLevel(t *testing.T) {
	var file bytes.Buffer
	ring := NewRingBuffer(10)
	handler := newHandler(&file, ring, slog.LevelInfo)
	logger := slog.New(handler)

	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("handler enabled DEBUG at INFO level")
	}
	logger.Debug("hidden")
	logger.Info("visible")

	if strings.Contains(file.String(), "hidden") {
		t.Fatalf("file output contains DEBUG record: %q", file.String())
	}
	if !strings.Contains(file.String(), "visible") {
		t.Fatalf("file output does not contain INFO record: %q", file.String())
	}
	lines := ring.Snapshot()
	if len(lines) != 1 || !strings.Contains(lines[0], "visible") {
		t.Fatalf("ring lines = %v, want only INFO record", lines)
	}
}

func TestNewCreatesLoggerAndRingBuffer(t *testing.T) {
	dir := t.TempDir()
	logger, ring, err := New(dir, slog.LevelInfo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	logger.Info("ready")

	data, err := os.ReadFile(filepath.Join(dir, defaultLogName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), `"msg":"ready"`) {
		t.Fatalf("log output = %q, want ready message", data)
	}
	if lines := ring.Snapshot(); len(lines) != 1 || !strings.Contains(lines[0], "ready") {
		t.Fatalf("ring lines = %v, want ready message", lines)
	}
}
