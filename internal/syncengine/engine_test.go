package syncengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh_gun/internal/config"
)

func TestChangePathsSummary(t *testing.T) {
	got := changePathsSummary([]Change{
		{Op: Upload, RelPath: "a.php"},
		{Op: Delete, RelPath: "b.php"},
	}, 20)
	want := "upload:a.php, delete:b.php"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	many := make([]Change, 0, 25)
	for i := 0; i < 25; i++ {
		many = append(many, Change{Op: Upload, RelPath: "f"})
	}
	got = changePathsSummary(many, 3)
	if !strings.Contains(got, "…(+22)") {
		t.Fatalf("expected truncation, got %q", got)
	}
}

func TestEngine_FullSyncRejectsConcurrentManualSync(t *testing.T) {
	backend := newRecordingBackend()
	backend.blockFullSync = make(chan struct{})
	engine := NewEngine(backend)
	mapping := testMapping(t, "manual-dup")
	mapping.ID = "map-manual"

	done := make(chan struct{})
	go func() {
		_, err := engine.FullSync(context.Background(), mapping)
		if err != nil {
			t.Errorf("first FullSync: %v", err)
		}
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	_, err := engine.FullSync(context.Background(), mapping)
	if !errors.Is(err, ErrSyncInProgress) {
		t.Fatalf("second FullSync err = %v, want ErrSyncInProgress", err)
	}

	close(backend.blockFullSync)
	<-done
}

func TestEngine_FullSyncRejectsWhileAutoPushRunning(t *testing.T) {
	backend := newRecordingBackend()
	backend.blockFirst = make(chan struct{})
	engine := NewEngine(backend)
	mapping := testMapping(t, "manual-auto")
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })

	if err := os.WriteFile(filepath.Join(mapping.LocalPath, "busy.txt"), []byte("busy"), 0o600); err != nil {
		t.Fatal(err)
	}
	receivePush(t, backend.calls)

	_, err := engine.FullSync(context.Background(), mapping)
	if !errors.Is(err, ErrSyncInProgress) {
		t.Fatalf("FullSync during auto push err = %v, want ErrSyncInProgress", err)
	}

	close(backend.blockFirst)
}

func TestEngine_FileCreatePushesUpload(t *testing.T) {
	backend := newRecordingBackend()
	engine := NewEngine(backend)
	mapping := testMapping(t, "create")
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })

	if err := os.WriteFile(filepath.Join(mapping.LocalPath, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	call := receivePush(t, backend.calls)
	assertChange(t, call, Change{Op: Upload, RelPath: "hello.txt", Size: 5})
}

func TestEngine_DebounceMergesRapidWritesIntoOnePush(t *testing.T) {
	backend := newRecordingBackend()
	engine := NewEngine(backend)
	mapping := testMapping(t, "debounce")
	mapping.DebounceMs = 50
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })
	path := filepath.Join(mapping.LocalPath, "rapid.txt")

	for _, content := range []string{"first", "second", "last"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	call := receivePush(t, backend.calls)
	if len(call) != 1 {
		t.Fatalf("PushChanges changes = %#v, want one merged change", call)
	}
	select {
	case extra := <-backend.calls:
		t.Fatalf("unexpected extra PushChanges call: %#v", extra)
	case <-time.After(120 * time.Millisecond):
	}
}

func TestEngine_MappingExcludesSkipConfiguredPaths(t *testing.T) {
	backend := newRecordingBackend()
	engine := NewEngine(backend)
	mapping := testMapping(t, "exclude")
	mapping.Excludes = []string{"node_modules"}
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })

	excluded := filepath.Join(mapping.LocalPath, "node_modules")
	if err := os.Mkdir(excluded, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excluded, "ignored.js"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-backend.calls:
		t.Fatalf("excluded path was pushed: %#v", call)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestEngine_UseGitignoreSkipsIgnoredFiles(t *testing.T) {
	backend := newRecordingBackend()
	engine := NewEngine(backend)
	mapping := testMapping(t, "gitignore")
	mapping.UseGitignore = true
	if err := os.WriteFile(filepath.Join(mapping.LocalPath, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })

	if err := os.WriteFile(filepath.Join(mapping.LocalPath, "ignored.txt"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-backend.calls:
		t.Fatalf("gitignore path was pushed: %#v", call)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestEngine_DirtyMappingRunsAgainAfterPushReturns(t *testing.T) {
	backend := newRecordingBackend()
	backend.blockFirst = make(chan struct{})
	engine := NewEngine(backend)
	mapping := testMapping(t, "dirty")
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })

	first := filepath.Join(mapping.LocalPath, "first.txt")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertChange(t, receivePush(t, backend.calls), Change{Op: Upload, RelPath: "first.txt"})

	second := filepath.Join(mapping.LocalPath, "second.txt")
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	close(backend.blockFirst)

	assertChange(t, receivePush(t, backend.calls), Change{Op: Upload, RelPath: "second.txt"})
}

func TestEngine_StopMappingCancelsPushAndPreventsFurtherPushes(t *testing.T) {
	backend := newRecordingBackend()
	backend.blockFirst = make(chan struct{})
	engine := NewEngine(backend)
	mapping := testMapping(t, "stop")
	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(mapping.LocalPath, "first.txt"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	receivePush(t, backend.calls)
	if err := engine.StopMapping(mapping.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight PushChanges was not canceled")
	}

	if err := os.WriteFile(filepath.Join(mapping.LocalPath, "later.txt"), []byte("later"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-backend.calls:
		t.Fatalf("push after StopMapping: %#v", call)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestEngine_ResolvesServerAndBuildsBackend(t *testing.T) {
	backend := newRecordingBackend()
	var gotServer config.Server
	var gotMapping *config.SyncMapping
	engine := NewEngineWithFactory(
		func(id string) (config.Server, error) {
			if id != "server-1" {
				return config.Server{}, errors.New("unexpected server")
			}
			return config.Server{ID: id, Host: "example.test"}, nil
		},
		func(_ context.Context, server config.Server, mapping *config.SyncMapping) (Backend, error) {
			gotServer = server
			gotMapping = mapping
			return backend, nil
		},
	)
	mapping := testMapping(t, "factory")
	mapping.ServerID = "server-1"
	mapping.Backend = "rsync"

	if err := engine.StartMapping(mapping); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.StopMapping(mapping.ID) })
	if gotServer.Host != "example.test" {
		t.Fatalf("factory server = %+v", gotServer)
	}
	if gotMapping == nil || gotMapping.Backend != "rsync" {
		t.Fatalf("factory mapping = %+v", gotMapping)
	}
}

func testMapping(t *testing.T, id string) *config.SyncMapping {
	t.Helper()
	return &config.SyncMapping{
		ID:         id,
		LocalPath:  t.TempDir(),
		RemotePath: "/remote",
		DebounceMs: 20,
	}
}

type recordingBackend struct {
	mu            sync.Mutex
	pushes        int
	calls         chan []Change
	canceled      chan struct{}
	blockFirst    chan struct{}
	blockFullSync chan struct{}
	cancelOnce    sync.Once
}

func newRecordingBackend() *recordingBackend {
	return &recordingBackend{
		calls:    make(chan []Change, 10),
		canceled: make(chan struct{}),
	}
}

func (b *recordingBackend) Name() string { return "recording" }

func (b *recordingBackend) FullSync(context.Context, *config.SyncMapping, func(Event)) (*Stats, error) {
	if b.blockFullSync != nil {
		<-b.blockFullSync
	}
	return &Stats{}, nil
}

func (b *recordingBackend) PushChanges(ctx context.Context, _ *config.SyncMapping, changes []Change, _ func(Event)) (*Stats, error) {
	copied := append([]Change(nil), changes...)
	b.mu.Lock()
	b.pushes++
	call := b.pushes
	b.mu.Unlock()
	b.calls <- copied
	if call == 1 && b.blockFirst != nil {
		select {
		case <-b.blockFirst:
		case <-ctx.Done():
			b.cancelOnce.Do(func() { close(b.canceled) })
			return nil, ctx.Err()
		}
	}
	return &Stats{}, nil
}

func receivePush(t *testing.T, calls <-chan []Change) []Change {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for PushChanges")
		return nil
	}
}
