package applog

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
)

type loggingHandler struct {
	inner slog.Handler
}

func newHandler(writer io.Writer, ring *RingBuffer, level slog.Level) slog.Handler {
	sink := &ringWriter{writer: writer, ring: ring}
	return &loggingHandler{
		inner: slog.NewJSONHandler(sink, &slog.HandlerOptions{Level: level}),
	}
}

func (h *loggingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *loggingHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.inner.Handle(ctx, record)
}

func (h *loggingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &loggingHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *loggingHandler) WithGroup(name string) slog.Handler {
	return &loggingHandler{inner: h.inner.WithGroup(name)}
}

type ringWriter struct {
	mu     sync.Mutex
	writer io.Writer
	ring   *RingBuffer
}

func (w *ringWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.writer.Write(p)
	if err != nil {
		return n, err
	}
	if w.ring != nil {
		line := strings.TrimSuffix(string(p[:n]), "\n")
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			w.ring.Append(line)
		}
	}
	return n, nil
}
