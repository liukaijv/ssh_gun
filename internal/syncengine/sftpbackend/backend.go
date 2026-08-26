package sftpbackend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"ssh_gun/internal/config"
	"ssh_gun/internal/sshclient"
	"ssh_gun/internal/syncengine"
)

const (
	uploadWorkers  = 4
	copyBufferSize = 64 * 1024
)

// Backend synchronizes files through SFTP and uses SSH exec for fast inventory
// collection when the remote supports GNU find.
type Backend struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

func New(client *ssh.Client) (*Backend, error) {
	if client == nil {
		return nil, errors.New("ssh client is required")
	}
	sftpClient, err := sshclient.NewSFTP(client)
	if err != nil {
		return nil, fmt.Errorf("open sftp: %w", err)
	}
	return &Backend{ssh: client, sftp: sftpClient}, nil
}

func (b *Backend) Close() error {
	if b == nil || b.sftp == nil {
		return nil
	}
	return b.sftp.Close()
}

func (b *Backend) Name() string { return "sftp" }

func (b *Backend) FullSync(ctx context.Context, mapping *config.SyncMapping, emit func(syncengine.Event)) (*syncengine.Stats, error) {
	if err := validateMapping(mapping); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	matcher, err := syncengine.NewExcludeMatcher(mapping)
	if err != nil {
		return nil, fmt.Errorf("build excludes: %w", err)
	}
	local, err := syncengine.ScanLocal(mapping.LocalPath, matcher)
	if err != nil {
		return nil, fmt.Errorf("scan local: %w", err)
	}
	if err := b.sftp.MkdirAll(mapping.RemotePath); err != nil {
		return nil, fmt.Errorf("create remote root: %w", err)
	}
	remote, err := b.listRemote(ctx, mapping, matcher)
	if err != nil {
		return nil, err
	}
	diff := syncengine.Diff(local, remote, mapping.CompareMode, time.Second)
	changes := make([]syncengine.Change, 0, len(diff.Delete)+len(diff.Mkdir)+len(diff.Upload))
	for _, entry := range selectedDeletes(diff, mapping.DeleteExtra) {
		changes = append(changes, changeFromEntry(syncengine.Delete, entry))
	}
	for _, entry := range diff.Mkdir {
		changes = append(changes, changeFromEntry(syncengine.Mkdir, entry))
	}
	for _, entry := range diff.Upload {
		changes = append(changes, changeFromEntry(syncengine.Upload, entry))
	}
	return b.PushChanges(ctx, mapping, changes, emit)
}

func (b *Backend) PushChanges(ctx context.Context, mapping *config.SyncMapping, changes []syncengine.Change, emit func(syncengine.Event)) (*syncengine.Stats, error) {
	if err := validateMapping(mapping); err != nil {
		return nil, err
	}
	stats := &syncengine.Stats{Started: time.Now()}
	defer func() { stats.Finished = time.Now() }()
	safeEmit := serializedEmitter(emit)
	matcher, err := syncengine.NewExcludeMatcher(mapping)
	if err != nil {
		return stats, fmt.Errorf("build excludes: %w", err)
	}

	var deletes, directories, uploads, chtimes []syncengine.Change
	for _, change := range changes {
		if _, err := cleanRel(change.RelPath); err != nil {
			return stats, err
		}
		if change.Op != syncengine.Delete && matcher != nil && matcher.ShouldSkip(change.RelPath, change.IsDir) {
			continue
		}
		if change.Op != syncengine.Delete && syncengine.IsExcludedVCSPath(change.RelPath) {
			continue
		}
		switch change.Op {
		case syncengine.Delete:
			deletes = append(deletes, change)
		case syncengine.Mkdir:
			directories = append(directories, change)
		case syncengine.Upload:
			uploads = append(uploads, change)
		case syncengine.Chtimes:
			chtimes = append(chtimes, change)
		default:
			return stats, fmt.Errorf("unsupported change operation %q", change.Op)
		}
	}

	sort.Slice(deletes, func(i, j int) bool { return pathDepth(deletes[i].RelPath) > pathDepth(deletes[j].RelPath) })
	for _, change := range deletes {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		remote, _ := b.remotePath(mapping, change.RelPath)
		err := b.sftp.RemoveAll(remote)
		if err != nil && !isNotExist(err) {
			safeEmit(syncengine.Event{Op: change.Op, RelPath: change.RelPath, Err: err})
			return stats, fmt.Errorf("delete %s: %w", change.RelPath, err)
		}
		stats.Deleted++
		safeEmit(syncengine.Event{Op: change.Op, RelPath: change.RelPath})
	}

	sort.Slice(directories, func(i, j int) bool { return pathDepth(directories[i].RelPath) < pathDepth(directories[j].RelPath) })
	for _, change := range directories {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		local := filepath.Join(mapping.LocalPath, filepath.FromSlash(change.RelPath))
		if syncengine.IsLocalReparsePoint(local) {
			continue
		}
		remote, _ := b.remotePath(mapping, change.RelPath)
		if err := b.sftp.MkdirAll(remote); err != nil {
			safeEmit(syncengine.Event{Op: change.Op, RelPath: change.RelPath, Err: err})
			return stats, fmt.Errorf("mkdir %s: %w", change.RelPath, err)
		}
		if mapping.DirMode != 0 {
			if err := b.sftp.Chmod(remote, os.FileMode(mapping.DirMode)); err != nil {
				return stats, fmt.Errorf("chmod directory %s: %w", change.RelPath, err)
			}
		}
		stats.Dirs++
		safeEmit(syncengine.Event{Op: change.Op, RelPath: change.RelPath})
	}

	if err := b.uploadAll(ctx, mapping, uploads, stats, safeEmit); err != nil {
		return stats, err
	}
	for _, change := range chtimes {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		remote, _ := b.remotePath(mapping, change.RelPath)
		if err := b.sftp.Chtimes(remote, change.ModTime, change.ModTime); err != nil {
			return stats, fmt.Errorf("chtimes %s: %w", change.RelPath, err)
		}
		safeEmit(syncengine.Event{Op: change.Op, RelPath: change.RelPath})
	}
	return stats, nil
}

func (b *Backend) uploadAll(ctx context.Context, mapping *config.SyncMapping, changes []syncengine.Change, stats *syncengine.Stats, emit func(syncengine.Event)) error {
	if len(changes) == 0 {
		return nil
	}
	workers := uploadWorkers
	if len(changes) < workers {
		workers = len(changes)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan syncengine.Change)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	var statsMu sync.Mutex
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for change := range jobs {
				bytes, err := b.uploadOne(workCtx, mapping, change, emit)
				if err != nil {
					select {
					case errs <- err:
						cancel()
					default:
					}
					return
				}
				statsMu.Lock()
				stats.Files++
				stats.Bytes += bytes
				statsMu.Unlock()
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, change := range changes {
			select {
			case jobs <- change:
			case <-workCtx.Done():
				return
			}
		}
	}()
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		return ctx.Err()
	}
}

func (b *Backend) uploadOne(ctx context.Context, mapping *config.SyncMapping, change syncengine.Change, emit func(syncengine.Event)) (int64, error) {
	rel, _ := cleanRel(change.RelPath)
	local := filepath.Join(mapping.LocalPath, filepath.FromSlash(rel))
	remote, _ := b.remotePath(mapping, rel)
	if syncengine.IsLocalReparsePoint(local) {
		return 0, nil
	}
	source, err := os.Open(local)
	if err != nil {
		if syncengine.IsInaccessibleLocalError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("open local %s: %w", rel, err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat local %s: %w", rel, err)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := b.sftp.MkdirAll(path.Dir(remote)); err != nil {
		return 0, fmt.Errorf("create parent for %s: %w", rel, err)
	}
	target, err := b.sftp.Create(remote)
	if err != nil {
		return 0, fmt.Errorf("create remote %s: %w", rel, err)
	}
	transferred, copyErr := copyWithContext(ctx, target, source, func(written int64) {
		emit(syncengine.Event{Op: syncengine.Upload, RelPath: rel, Transferred: written, Total: info.Size()})
	})
	closeErr := target.Close()
	if copyErr != nil {
		emit(syncengine.Event{Op: syncengine.Upload, RelPath: rel, Transferred: transferred, Total: info.Size(), Err: copyErr})
		return transferred, copyErr
	}
	if closeErr != nil {
		return transferred, fmt.Errorf("close remote %s: %w", rel, closeErr)
	}
	mode := info.Mode().Perm()
	if mapping.FileMode != 0 {
		mode = os.FileMode(mapping.FileMode)
	}
	if err := b.sftp.Chmod(remote, mode); err != nil {
		return transferred, fmt.Errorf("chmod %s: %w", rel, err)
	}
	modTime := change.ModTime
	if modTime.IsZero() {
		modTime = info.ModTime()
	}
	if err := b.sftp.Chtimes(remote, modTime, modTime); err != nil {
		return transferred, fmt.Errorf("chtimes %s: %w", rel, err)
	}
	return transferred, nil
}

func (b *Backend) listRemote(ctx context.Context, mapping *config.SyncMapping, matcher *syncengine.ExcludeMatcher) ([]syncengine.FileEntry, error) {
	command := "cd -- " + syncengine.POSIXShellQuote(mapping.RemotePath) +
		" && find . -mindepth 1 -printf '%y\\t%P\\t%s\\t%T@\\n'"
	if output, err := sshclient.Exec(ctx, b.ssh, command); err == nil {
		if entries, parseErr := syncengine.ParseFindPrintf(output); parseErr == nil {
			return filterExcludes(entries, matcher), nil
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return b.walkRemote(ctx, mapping, matcher)
}

func (b *Backend) walkRemote(ctx context.Context, mapping *config.SyncMapping, matcher *syncengine.ExcludeMatcher) ([]syncengine.FileEntry, error) {
	walker := b.sftp.Walk(mapping.RemotePath)
	entries := make([]syncengine.FileEntry, 0)
	for walker.Step() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := walker.Err(); err != nil {
			return nil, fmt.Errorf("walk remote: %w", err)
		}
		rel := remoteRel(mapping.RemotePath, walker.Path())
		if rel == "." || rel == "" {
			continue
		}
		rel = strings.TrimPrefix(rel, "./")
		info := walker.Stat()
		isDir := info != nil && info.IsDir()
		if matcher != nil && matcher.ShouldSkip(rel, isDir) {
			continue
		}
		entries = append(entries, syncengine.FileEntry{
			RelPath: rel, Size: info.Size(), ModTime: info.ModTime(), IsDir: isDir,
		})
	}
	return entries, nil
}

func (b *Backend) remotePath(mapping *config.SyncMapping, rel string) (string, error) {
	clean, err := cleanRel(rel)
	if err != nil {
		return "", err
	}
	return path.Join(mapping.RemotePath, clean), nil
}

func validateMapping(mapping *config.SyncMapping) error {
	if mapping == nil {
		return errors.New("sync mapping is required")
	}
	if mapping.LocalPath == "" {
		return errors.New("local path is required")
	}
	if mapping.RemotePath == "" {
		return errors.New("remote path is required")
	}
	return nil
}

func cleanRel(rel string) (string, error) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	clean := path.Clean(rel)
	if clean == "." || clean == "" || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid relative path %q", rel)
	}
	return clean, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, progress func(int64)) (int64, error) {
	buffer := make([]byte, copyBufferSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			written, writeErr := dst.Write(buffer[:n])
			total += int64(written)
			progress(total)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func selectedDeletes(diff syncengine.Changes, deleteExtra bool) []syncengine.FileEntry {
	if deleteExtra {
		return diff.Delete
	}
	replacements := make(map[string]struct{}, len(diff.Mkdir)+len(diff.Upload))
	for _, entry := range diff.Mkdir {
		replacements[entry.RelPath] = struct{}{}
	}
	for _, entry := range diff.Upload {
		replacements[entry.RelPath] = struct{}{}
	}
	selected := make([]syncengine.FileEntry, 0)
	for _, entry := range diff.Delete {
		if _, replacing := replacements[entry.RelPath]; replacing {
			selected = append(selected, entry)
		}
	}
	return selected
}

func changeFromEntry(op syncengine.ChangeOp, entry syncengine.FileEntry) syncengine.Change {
	return syncengine.Change{
		Op: op, RelPath: entry.RelPath, Size: entry.Size, ModTime: entry.ModTime, IsDir: entry.IsDir,
	}
}

func filterExcludes(entries []syncengine.FileEntry, matcher *syncengine.ExcludeMatcher) []syncengine.FileEntry {
	filtered := entries[:0]
	for _, entry := range entries {
		if matcher == nil || !matcher.ShouldSkip(entry.RelPath, entry.IsDir) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func serializedEmitter(emit func(syncengine.Event)) func(syncengine.Event) {
	if emit == nil {
		return func(syncengine.Event) {}
	}
	var mu sync.Mutex
	return func(event syncengine.Event) {
		mu.Lock()
		defer mu.Unlock()
		emit(event)
	}
}

func pathDepth(rel string) int {
	return strings.Count(strings.Trim(rel, "/"), "/")
}

func remoteRel(root, current string) string {
	root = strings.TrimSuffix(path.Clean(root), "/")
	current = path.Clean(current)
	if current == root {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(current, root), "/")
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "no such file")
}

var _ syncengine.Backend = (*Backend)(nil)
