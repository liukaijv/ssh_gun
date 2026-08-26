package syncengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/fsnotify/fsnotify"
)

const defaultDebounce = 250 * time.Millisecond

// Watcher recursively watches a local root and emits debounced change batches.
type Watcher struct {
	root     string
	matcher  *ExcludeMatcher
	debounce time.Duration
	now      func() time.Time
}

// NewWatcher creates a watcher. The filesystem watches are installed by Watch.
func NewWatcher(root string, matcher *ExcludeMatcher, debounce time.Duration, now func() time.Time) (*Watcher, error) {
	if root == "" {
		return nil, errors.New("watch root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve watch root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat watch root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("watch root %q is not a directory", absolute)
	}
	if debounce <= 0 {
		debounce = defaultDebounce
	}
	if now == nil {
		now = time.Now
	}
	if matcher == nil {
		matcher = &ExcludeMatcher{}
	}
	return &Watcher{
		root:     filepath.Clean(absolute),
		matcher:  matcher,
		debounce: debounce,
		now:      now,
	}, nil
}

// Watch installs recursive watches synchronously, then emits changes until ctx
// is canceled. The returned channels are closed when watching stops.
func (w *Watcher) Watch(ctx context.Context) (<-chan []Change, <-chan error, error) {
	native, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, fmt.Errorf("create filesystem watcher: %w", err)
	}
	if err := w.addRecursive(native, w.root, nil); err != nil {
		_ = native.Close()
		return nil, nil, err
	}

	batches := make(chan []Change, 1)
	errs := make(chan error, 1)
	go w.run(ctx, native, batches, errs)
	return batches, errs, nil
}

func (w *Watcher) run(
	ctx context.Context,
	native *fsnotify.Watcher,
	batches chan<- []Change,
	errs chan<- error,
) {
	defer close(batches)
	defer close(errs)
	defer native.Close()

	pending := make(map[string]struct{})
	var lastEvent time.Time
	var timer *time.Timer
	var timerC <-chan time.Time

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(w.debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)
		}
		timerC = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-native.Errors:
			if !ok {
				return
			}
			select {
			case errs <- err:
			default:
			}
		case event, ok := <-native.Events:
			if !ok {
				return
			}
			rel, err := ToSlashRel(w.root, event.Name)
			if err != nil || rel == "" || w.matcher.ShouldSkipUnknown(rel) {
				continue
			}
			pending[rel] = struct{}{}
			if info, statErr := os.Lstat(event.Name); statErr == nil && info.IsDir() {
				if err := w.addRecursive(native, event.Name, pending); err != nil {
					select {
					case errs <- err:
					default:
					}
				}
			}
			lastEvent = w.now()
			resetTimer()
		case <-timerC:
			elapsed := w.now().Sub(lastEvent)
			if elapsed < w.debounce {
				timer.Reset(w.debounce - elapsed)
				timerC = timer.C
				continue
			}
			changes := w.resolve(pending)
			clear(pending)
			timerC = nil
			if len(changes) == 0 {
				continue
			}
			select {
			case batches <- changes:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *Watcher) addRecursive(native *fsnotify.Watcher, root string, discovered map[string]struct{}) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if isVCSMetaDir(entry.Name()) {
			return filepath.SkipDir
		}
		rel, err := ToSlashRel(w.root, path)
		if err != nil {
			return err
		}
		if rel != "" && w.matcher.ShouldSkip(rel, true) {
			return filepath.SkipDir
		}
		if isReparsePoint(entry, path) {
			return filepath.SkipDir
		}
		if err := native.Add(path); err != nil {
			return fmt.Errorf("watch directory %q: %w", path, err)
		}
		if discovered != nil && rel != "" {
			discovered[rel] = struct{}{}
		}
		return nil
	})
}

func (w *Watcher) resolve(pending map[string]struct{}) []Change {
	changes := make([]Change, 0, len(pending))
	for rel := range pending {
		absolute := filepath.Join(w.root, filepath.FromSlash(rel))
		info, err := os.Lstat(absolute)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if !w.matcher.ShouldSkipUnknown(rel) {
					changes = append(changes, Change{Op: Delete, RelPath: rel})
				}
			}
			continue
		}
		if w.matcher.ShouldSkip(rel, info.IsDir()) {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		change := Change{
			Op:      Upload,
			RelPath: rel,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		}
		if info.IsDir() {
			change.Op = Mkdir
		}
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].RelPath == changes[j].RelPath {
			return changes[i].Op < changes[j].Op
		}
		return changes[i].RelPath < changes[j].RelPath
	})
	return changes
}
