package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ssh_gun/internal/config"
)

func TestWatcher_FileCreateEmitsUpload(t *testing.T) {
	root := t.TempDir()
	batches := startTestWatcher(t, root, &ExcludeMatcher{}, 20*time.Millisecond, time.Now)

	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	batch := receiveBatch(t, batches)
	assertChange(t, batch, Change{Op: Upload, RelPath: "hello.txt", Size: 5})
}

func TestWatcher_DebouncesRapidWrites(t *testing.T) {
	root := t.TempDir()
	batches := startTestWatcher(t, root, nil, 40*time.Millisecond, time.Now)
	path := filepath.Join(root, "rapid.txt")

	for _, content := range []string{"one", "two", "final"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	batch := receiveBatch(t, batches)
	if len(batch) != 1 {
		t.Fatalf("batch = %#v, want one merged change", batch)
	}
	assertChange(t, batch, Change{Op: Upload, RelPath: "rapid.txt", Size: 5})
	select {
	case extra := <-batches:
		t.Fatalf("unexpected second batch: %#v", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatcher_IgnoresExcludedDirectory(t *testing.T) {
	root := t.TempDir()
	excluded := filepath.Join(root, "node_modules")
	if err := os.Mkdir(excluded, 0o700); err != nil {
		t.Fatal(err)
	}
	batches := startTestWatcher(t, root, mustMatcher(t, &config.SyncMapping{
		LocalPath: root,
		Excludes:  []string{"node_modules"},
	}), 20*time.Millisecond, time.Now)

	if err := os.WriteFile(filepath.Join(excluded, "ignored.js"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case batch := <-batches:
		t.Fatalf("excluded write emitted changes: %#v", batch)
	case <-time.After(100 * time.Millisecond):
	}

	if err := os.WriteFile(filepath.Join(root, "included.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertChange(t, receiveBatch(t, batches), Change{Op: Upload, RelPath: "included.txt"})
}

func TestWatcher_IgnoresGitignorePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := NewExcludeMatcher(&config.SyncMapping{LocalPath: root, UseGitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	batches := startTestWatcher(t, root, matcher, 20*time.Millisecond, time.Now)

	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case batch := <-batches:
		t.Fatalf("gitignore path emitted changes: %#v", batch)
	case <-time.After(100 * time.Millisecond):
	}

	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("yes"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertChange(t, receiveBatch(t, batches), Change{Op: Upload, RelPath: "ok.txt"})
}

func TestWatcher_AddsNewDirectoriesRecursively(t *testing.T) {
	root := t.TempDir()
	batches := startTestWatcher(t, root, nil, 100*time.Millisecond, time.Now)
	nested := filepath.Join(root, "new", "nested")

	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	// Give fsnotify time to process the directory event and install the watch.
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	batch := receiveBatch(t, batches)
	assertChange(t, batch, Change{Op: Mkdir, RelPath: "new"})
	assertChange(t, batch, Change{Op: Mkdir, RelPath: "new/nested"})
	assertChange(t, batch, Change{Op: Upload, RelPath: "new/nested/file.txt"})
}

func TestWatcher_UsesInjectedClockForDebounce(t *testing.T) {
	root := t.TempDir()
	clock := &fakeClock{current: time.Unix(100, 0)}
	batches := startTestWatcher(t, root, nil, 20*time.Millisecond, clock.Now)

	if err := os.WriteFile(filepath.Join(root, "clock.txt"), []byte("tick"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case batch := <-batches:
		t.Fatalf("batch emitted before clock advanced: %#v", batch)
	case <-time.After(50 * time.Millisecond):
	}

	clock.Advance(20 * time.Millisecond)
	assertChange(t, receiveBatch(t, batches), Change{Op: Upload, RelPath: "clock.txt"})
}

func mustMatcher(t *testing.T, mapping *config.SyncMapping) *ExcludeMatcher {
	t.Helper()
	matcher, err := NewExcludeMatcher(mapping)
	if err != nil {
		t.Fatal(err)
	}
	return matcher
}

func startTestWatcher(
	t *testing.T,
	root string,
	matcher *ExcludeMatcher,
	debounce time.Duration,
	now func() time.Time,
) <-chan []Change {
	t.Helper()
	watcher, err := NewWatcher(root, matcher, debounce, now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	batches, errs, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for err := range errs {
			if err != nil && ctx.Err() == nil {
				t.Errorf("watch error: %v", err)
			}
		}
	}()
	return batches
}

func receiveBatch(t *testing.T, batches <-chan []Change) []Change {
	t.Helper()
	select {
	case batch := <-batches:
		return batch
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watcher batch")
		return nil
	}
}

func assertChange(t *testing.T, changes []Change, want Change) {
	t.Helper()
	for _, got := range changes {
		if got.Op == want.Op && got.RelPath == want.RelPath &&
			(want.Size == 0 || got.Size == want.Size) {
			return
		}
	}
	t.Fatalf("changes = %#v, want change %#v", changes, want)
}

type fakeClock struct {
	mu      sync.Mutex
	current time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(duration)
}
