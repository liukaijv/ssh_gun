package syncengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"ssh_gun/internal/config"
)

// ErrSyncInProgress is returned when a mapping already has a sync running.
var ErrSyncInProgress = errors.New("sync already in progress")

// GetServerFunc resolves a configured server by ID.
type GetServerFunc func(id string) (config.Server, error)

// BackendFactory constructs the backend selected by a synchronization mapping.
type BackendFactory func(ctx context.Context, server config.Server, mapping *config.SyncMapping) (Backend, error)

// EngineOption customizes an Engine.
type EngineOption func(*Engine)

// WithClock sets the clock used by mapping watchers.
func WithClock(now func() time.Time) EngineOption {
	return func(engine *Engine) {
		if now != nil {
			engine.now = now
		}
	}
}

// WithEmitter sets the callback used for backend progress and engine errors.
func WithEmitter(emit func(Event)) EngineOption {
	return func(engine *Engine) {
		if emit != nil {
			engine.emit = emit
		}
	}
}

// SyncFinish describes the outcome of an auto or manual sync pass for UI status.
type SyncFinish struct {
	MappingID string
	Result    string // ok | failed
	Error     string
	At        time.Time
}

// WithSyncFinish is called when an auto-sync PushChanges finishes (success or fail).
func WithSyncFinish(fn func(SyncFinish)) EngineOption {
	return func(engine *Engine) {
		if fn != nil {
			engine.onFinish = fn
		}
	}
}

// Engine runs independent, serial auto-sync queues for configured mappings.
type Engine struct {
	backend  Backend
	get      GetServerFunc
	factory  BackendFactory
	now      func() time.Time
	emit     func(Event)
	onFinish func(SyncFinish)

	mu         sync.Mutex
	mappings   map[string]*runningMapping
	syncMu     sync.Mutex
	activeSync map[string]struct{}
}

type runningMapping struct {
	mapping *config.SyncMapping
	backend Backend
	owned   bool
	cancel  context.CancelFunc
	done    chan struct{}
	retry   chan struct{}
}

type pushResult struct {
	mappingID   string
	stats       *Stats
	err         error
	started     time.Time
	name        string
	backendName string
}

// NewEngine creates an engine using an already-constructed backend.
func NewEngine(backend Backend, options ...EngineOption) *Engine {
	engine := newEngine(options)
	engine.backend = backend
	return engine
}

// NewEngineWithFactory creates an engine that resolves and dials each mapping's
// server when StartMapping is called.
func NewEngineWithFactory(get GetServerFunc, factory BackendFactory, options ...EngineOption) *Engine {
	engine := newEngine(options)
	engine.get = get
	engine.factory = factory
	return engine
}

func newEngine(options []EngineOption) *Engine {
	engine := &Engine{
		now:        time.Now,
		emit:       func(Event) {},
		mappings:   make(map[string]*runningMapping),
		activeSync: make(map[string]struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(engine)
		}
	}
	return engine
}

// StartMapping begins watching mapping.LocalPath. A mapping ID can only have
// one active worker.
func (e *Engine) StartMapping(mapping *config.SyncMapping) error {
	if mapping == nil {
		return errors.New("sync mapping is required")
	}
	if mapping.ID == "" {
		return errors.New("sync mapping ID is required")
	}

	e.mu.Lock()
	if _, exists := e.mappings[mapping.ID]; exists {
		e.mu.Unlock()
		return fmt.Errorf("sync mapping %q is already running", mapping.ID)
	}
	e.mu.Unlock()

	copyMapping := *mapping
	copyMapping.Excludes = append([]string(nil), mapping.Excludes...)
	backend, owned, err := e.resolveBackend(context.Background(), &copyMapping)
	if err != nil {
		return err
	}
	matcher, err := NewExcludeMatcher(&copyMapping)
	if err != nil {
		closeOwnedBackend(backend, owned)
		return err
	}
	debounce := time.Duration(copyMapping.DebounceMs) * time.Millisecond
	watcher, err := NewWatcher(copyMapping.LocalPath, matcher, debounce, e.now)
	if err != nil {
		closeOwnedBackend(backend, owned)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	batches, watchErrors, err := watcher.Watch(ctx)
	if err != nil {
		cancel()
		closeOwnedBackend(backend, owned)
		return err
	}
	running := &runningMapping{
		mapping: &copyMapping,
		backend: backend,
		owned:   owned,
		cancel:  cancel,
		done:    make(chan struct{}),
		retry:   make(chan struct{}, 1),
	}

	e.mu.Lock()
	if _, exists := e.mappings[mapping.ID]; exists {
		e.mu.Unlock()
		cancel()
		closeOwnedBackend(backend, owned)
		return fmt.Errorf("sync mapping %q is already running", mapping.ID)
	}
	e.mappings[mapping.ID] = running
	e.mu.Unlock()

	go e.runMapping(ctx, running, batches, watchErrors)
	return nil
}

// StopMapping stops its watcher, cancels an in-flight push, and waits for the
// mapping worker to exit. Stopping an unknown ID is a no-op.
func (e *Engine) StopMapping(id string) error {
	e.mu.Lock()
	running, exists := e.mappings[id]
	if exists {
		delete(e.mappings, id)
	}
	e.mu.Unlock()
	if !exists {
		return nil
	}
	running.cancel()
	<-running.done
	return nil
}

// FullSync performs a one-off full synchronization using the engine backend
// configuration.
func (e *Engine) FullSync(ctx context.Context, mapping *config.SyncMapping) (*Stats, error) {
	if mapping == nil {
		return nil, errors.New("sync mapping is required")
	}
	if mapping.ID == "" {
		return nil, errors.New("sync mapping ID is required")
	}
	if err := e.beginSync(mapping.ID); err != nil {
		return nil, err
	}
	defer e.endSync(mapping.ID)

	backend, owned, err := e.resolveBackend(ctx, mapping)
	if err != nil {
		return nil, err
	}
	defer closeOwnedBackend(backend, owned)
	return backend.FullSync(ctx, mapping, e.emit)
}

func (e *Engine) beginSync(id string) error {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	if _, busy := e.activeSync[id]; busy {
		return ErrSyncInProgress
	}
	e.activeSync[id] = struct{}{}
	return nil
}

func (e *Engine) endSync(id string) {
	e.syncMu.Lock()
	delete(e.activeSync, id)
	var retry chan struct{}
	if running, ok := e.mappings[id]; ok {
		retry = running.retry
	}
	e.syncMu.Unlock()
	if retry != nil {
		select {
		case retry <- struct{}{}:
		default:
		}
	}
}

func (e *Engine) runMapping(
	ctx context.Context,
	running *runningMapping,
	batches <-chan []Change,
	watchErrors <-chan error,
) {
	defer close(running.done)
	defer closeOwnedBackend(running.backend, running.owned)

	pending := make(map[string]Change)
	results := make(chan pushResult, 1)
	syncing := false

	startPush := func() {
		if syncing || len(pending) == 0 || ctx.Err() != nil {
			return
		}
		if err := e.beginSync(running.mapping.ID); err != nil {
			return
		}
		changes := sortedChanges(pending)
		clear(pending)
		syncing = true
		name := running.mapping.Name
		if name == "" {
			name = running.mapping.ID
		}
		started := e.now()
		backendName := ""
		if running.backend != nil {
			backendName = running.backend.Name()
		}
		slog.Info("sync start",
			"mapping", name,
			"trigger", "auto",
			"backend", backendName,
			"files", len(changes),
			"paths", changePathsSummary(changes, 20),
		)
		go func() {
			defer e.endSync(running.mapping.ID)
			stats, err := running.backend.PushChanges(ctx, running.mapping, changes, e.emit)
			results <- pushResult{
				mappingID:   running.mapping.ID,
				stats:       stats,
				err:         err,
				started:     started,
				name:        name,
				backendName: backendName,
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			if syncing {
				<-results
			}
			return
		case <-running.retry:
			startPush()
		case batch, ok := <-batches:
			if !ok {
				batches = nil
				continue
			}
			for _, change := range batch {
				pending[change.RelPath] = change
			}
			startPush()
		case err, ok := <-watchErrors:
			if !ok {
				watchErrors = nil
				continue
			}
			if err != nil {
				e.emit(Event{Err: err})
			}
		case result := <-results:
			syncing = false
			if result.err != nil && ctx.Err() == nil {
				slog.Error("sync failed", "mapping", result.name, "trigger", "auto", "backend", result.backendName, "err", result.err)
				e.emit(Event{Err: result.err})
				if e.onFinish != nil {
					e.onFinish(SyncFinish{
						MappingID: result.mappingID,
						Result:    "failed",
						Error:     result.err.Error(),
						At:        e.now(),
					})
				}
			} else if result.err == nil {
				files, deleted, bytes := 0, 0, int64(0)
				if result.stats != nil {
					files = result.stats.Files
					deleted = result.stats.Deleted
					bytes = result.stats.Bytes
				}
				slog.Info("sync end",
					"mapping", result.name,
					"trigger", "auto",
					"backend", result.backendName,
					"files", files,
					"deleted", deleted,
					"bytes", bytes,
					"dur", e.now().Sub(result.started).String(),
				)
				if e.onFinish != nil {
					e.onFinish(SyncFinish{
						MappingID: result.mappingID,
						Result:    "ok",
						At:        e.now(),
					})
				}
			}
			startPush()
		}
	}
}

func changePathsSummary(changes []Change, limit int) string {
	if limit <= 0 {
		limit = 20
	}
	n := len(changes)
	if n == 0 {
		return ""
	}
	show := n
	if show > limit {
		show = limit
	}
	parts := make([]string, 0, show)
	for i := 0; i < show; i++ {
		op := string(changes[i].Op)
		if op == "" {
			op = "upload"
		}
		parts = append(parts, op+":"+changes[i].RelPath)
	}
	if n > limit {
		return strings.Join(parts, ", ") + fmt.Sprintf(" …(+%d)", n-limit)
	}
	return strings.Join(parts, ", ")
}

func (e *Engine) resolveBackend(ctx context.Context, mapping *config.SyncMapping) (Backend, bool, error) {
	if e.backend != nil {
		return e.backend, false, nil
	}
	if e.get == nil || e.factory == nil {
		return nil, false, errors.New("sync backend or server resolver and backend factory are required")
	}
	server, err := e.get(mapping.ServerID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve server %q: %w", mapping.ServerID, err)
	}
	backend, err := e.factory(ctx, server, mapping)
	if err != nil {
		return nil, false, fmt.Errorf("create backend for server %q: %w", mapping.ServerID, err)
	}
	if backend == nil {
		return nil, false, errors.New("backend factory returned nil")
	}
	return backend, true, nil
}

func sortedChanges(pending map[string]Change) []Change {
	changes := make([]Change, 0, len(pending))
	for _, change := range pending {
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].RelPath < changes[j].RelPath
	})
	return changes
}

func closeOwnedBackend(backend Backend, owned bool) {
	if !owned {
		return
	}
	if closer, ok := backend.(io.Closer); ok {
		_ = closer.Close()
	}
}
