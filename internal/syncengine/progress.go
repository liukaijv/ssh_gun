package syncengine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"ssh_gun/internal/config"
)

const (
	progressBatchSize = 50
	progressInterval  = 2 * time.Second
)

// MappingLogName is the mapping label used in sync progress logs.
func MappingLogName(mapping *config.SyncMapping) string {
	if mapping == nil {
		return ""
	}
	if mapping.Name != "" {
		return mapping.Name
	}
	return mapping.ID
}

// ProgressLog emits batched Info logs during a sync pass.
type ProgressLog struct {
	mapping  string
	batch    int
	interval time.Duration
	now      func() time.Time
	logf     func(string, ...any)

	mu         sync.Mutex
	phase      string
	files      int
	dirs       int
	deleted    int
	bytes      int64
	inProgress map[string]int64
	latest     string
	sinceLast  int
	lastLog    time.Time
}

// NewProgressLog starts a batching progress logger for mapping.
func NewProgressLog(mapping string) *ProgressLog {
	now := time.Now
	return &ProgressLog{
		mapping:    mapping,
		batch:      progressBatchSize,
		interval:   progressInterval,
		now:        now,
		logf:       slog.Info,
		inProgress: make(map[string]int64),
		lastLog:    now(),
	}
}

// Begin switches the current phase and logs immediately so long inventory
// stages are visible before any files are transferred.
func (p *ProgressLog) Begin(phase string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phase = phase
	p.files = 0
	p.dirs = 0
	p.deleted = 0
	p.bytes = 0
	p.latest = ""
	p.sinceLast = 0
	clear(p.inProgress)
	p.emitLocked()
}

// SetScan records inventory totals for a completed scan phase and logs once.
func (p *ProgressLog) SetScan(files, dirs int, latest string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.files = files
	p.dirs = dirs
	p.latest = latest
	p.emitLocked()
}

// Tick counts a scanned local or remote path and may emit a progress log.
func (p *ProgressLog) Tick(rel string, isDir bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if isDir {
		p.dirs++
	} else {
		p.files++
	}
	p.latest = rel
	p.sinceLast++
	p.maybeLogLocked()
}

// Record counts a completed delete, mkdir, or upload and may emit a progress log.
func (p *ProgressLog) Record(op ChangeOp, rel string, nbytes int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inProgress, rel)
	switch op {
	case Delete:
		p.deleted++
	case Mkdir:
		p.dirs++
	default:
		p.files++
		p.bytes += nbytes
	}
	p.latest = rel
	p.sinceLast++
	p.maybeLogLocked()
}

// NoteBytes records in-flight transfer bytes for a file still being uploaded.
// A progress log is emitted at most once per interval while the file is in flight.
func (p *ProgressLog) NoteBytes(rel string, transferred int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inProgress == nil {
		p.inProgress = make(map[string]int64)
	}
	p.inProgress[rel] = transferred
	p.latest = rel
	p.maybeLogLocked()
}

// Heartbeat re-emits the current phase if the interval has elapsed with no
// other progress. Used while blocked on a remote find.
func (p *ProgressLog) Heartbeat() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.now().Sub(p.lastLog) < p.interval {
		return
	}
	p.emitLocked()
}

// StartHeartbeat logs the current phase on interval until the returned stop
// function is called or ctx is cancelled.
func (p *ProgressLog) StartHeartbeat(ctx context.Context) func() {
	if p == nil {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(done) }) }
	interval := p.interval
	if interval <= 0 {
		interval = progressInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				p.Heartbeat()
			}
		}
	}()
	return stop
}

func (p *ProgressLog) maybeLogLocked() {
	now := p.now()
	inBytes := int64(0)
	for _, n := range p.inProgress {
		inBytes += n
	}
	batchHit := p.sinceLast >= p.batch
	intervalHit := now.Sub(p.lastLog) >= p.interval && (p.sinceLast > 0 || inBytes > 0)
	if !batchHit && !intervalHit {
		return
	}
	p.emitLocked()
}

func (p *ProgressLog) emitLocked() {
	inBytes := int64(0)
	for _, n := range p.inProgress {
		inBytes += n
	}
	args := []any{
		"mapping", p.mapping,
		"phase", p.phase,
		"files", p.files,
		"dirs", p.dirs,
		"deleted", p.deleted,
		"bytes", p.bytes + inBytes,
	}
	if p.latest != "" {
		args = append(args, "latest", p.latest)
	}
	p.logf("sync progress", args...)
	p.lastLog = p.now()
	p.sinceLast = 0
}
