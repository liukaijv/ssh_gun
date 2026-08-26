package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultMaxBytes   int64 = 10 * 1024 * 1024
	defaultRetainDays       = 7
)

// rotatingWriter appends to a log file and archives it before a write would
// push a non-empty file beyond maxBytes.
type rotatingWriter struct {
	mu         sync.Mutex
	path       string
	maxBytes   int64
	retainDays int
}

func newRotatingWriter(path string, maxBytes int64, retainDays int) (*rotatingWriter, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if retainDays <= 0 {
		retainDays = defaultRetainDays
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	w := &rotatingWriter{
		path:       path,
		maxBytes:   maxBytes,
		retainDays: retainDays,
	}
	if err := w.cleanupLocked(time.Now()); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, err := os.Stat(w.path)
	switch {
	case err == nil && info.Size() > 0 && info.Size()+int64(len(p)) > w.maxBytes:
		if err := w.rotateLocked(time.Now()); err != nil {
			return 0, err
		}
	case err != nil && !os.IsNotExist(err):
		return 0, fmt.Errorf("stat log file: %w", err)
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	n, writeErr := file.Write(p)
	closeErr := file.Close()
	if writeErr != nil {
		return n, fmt.Errorf("write log file: %w", writeErr)
	}
	if closeErr != nil {
		return n, fmt.Errorf("close log file: %w", closeErr)
	}
	return n, nil
}

func (w *rotatingWriter) rotateLocked(now time.Time) error {
	archive, err := w.nextArchivePathLocked(now)
	if err != nil {
		return err
	}
	if err := os.Rename(w.path, archive); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}
	if err := w.cleanupLocked(now); err != nil {
		return err
	}
	return nil
}

func (w *rotatingWriter) nextArchivePathLocked(now time.Time) (string, error) {
	ext := filepath.Ext(w.path)
	stem := w.path[:len(w.path)-len(ext)]
	base := stem + "-" + now.Format("20060102-150405")
	for suffix := 0; ; suffix++ {
		candidate := base + ext
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d%s", base, suffix, ext)
		}
		_, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("stat rotated log: %w", err)
		}
	}
}

func (w *rotatingWriter) cleanupLocked(now time.Time) error {
	ext := filepath.Ext(w.path)
	stem := w.path[:len(w.path)-len(ext)]
	matches, err := filepath.Glob(stem + "-*" + ext)
	if err != nil {
		return fmt.Errorf("find rotated logs: %w", err)
	}
	cutoff := now.Add(-time.Duration(w.retainDays) * 24 * time.Hour)
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat rotated log %q: %w", path, err)
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove rotated log %q: %w", path, err)
			}
		}
	}
	return nil
}
