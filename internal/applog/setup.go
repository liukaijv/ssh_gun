package applog

import (
	"fmt"
	"log/slog"
	"path/filepath"
)

const (
	defaultLogName      = "ssh_gun.log"
	defaultRingCapacity = 1_000
)

// New creates the application logger and its in-memory recent-line buffer.
func New(dir string, level slog.Level) (*slog.Logger, *RingBuffer, error) {
	writer, err := newRotatingWriter(
		filepath.Join(dir, defaultLogName),
		defaultMaxBytes,
		defaultRetainDays,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create application log: %w", err)
	}
	ring := NewRingBuffer(defaultRingCapacity)
	return slog.New(newHandler(writer, ring, level)), ring, nil
}
