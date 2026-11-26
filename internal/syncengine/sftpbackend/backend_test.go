package sftpbackend_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"ssh_gun/internal/config"
	"ssh_gun/internal/sshclient"
	"ssh_gun/internal/syncengine"
	"ssh_gun/internal/syncengine/sftpbackend"
	"ssh_gun/internal/testsshd"
)

func TestPushChanges_UploadsFileAndPreservesMtime(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	want := []byte("hello over sftp")
	writeLocal(t, local, "hello.txt", want)
	mtime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(filepath.Join(local, "hello.txt"), mtime, mtime); err != nil {
		t.Fatal(err)
	}

	stats, err := backend.PushChanges(context.Background(), mapping(local), []syncengine.Change{{
		Op: syncengine.Upload, RelPath: "hello.txt", Size: int64(len(want)), ModTime: mtime,
	}}, nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if stats.Files != 1 || stats.Bytes != int64(len(want)) {
		t.Fatalf("stats = %+v", stats)
	}
	got, err := os.ReadFile(filepath.Join(remote, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
	info, err := os.Stat(filepath.Join(remote, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(mtime) {
		t.Fatalf("mtime = %v, want %v", info.ModTime(), mtime)
	}
}

func TestPushChanges_CreatesNestedDirectories(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	if err := os.MkdirAll(filepath.Join(local, "one", "two"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := backend.PushChanges(context.Background(), mapping(local), []syncengine.Change{
		{Op: syncengine.Mkdir, RelPath: "one", IsDir: true},
		{Op: syncengine.Mkdir, RelPath: "one/two", IsDir: true},
	}, nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	info, err := os.Stat(filepath.Join(remote, "one", "two"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("nested path is not a directory")
	}
}

func TestDelete_RemovesRemoteFiles(t *testing.T) {
	t.Run("explicit change", func(t *testing.T) {
		local, remote, backend := newBackend(t, nil)
		writeRemote(t, remote, "obsolete.txt", []byte("old"))

		stats, err := backend.PushChanges(context.Background(), mapping(local), []syncengine.Change{{
			Op: syncengine.Delete, RelPath: "obsolete.txt",
		}}, nil)
		if err != nil {
			t.Fatalf("push: %v", err)
		}
		if stats.Deleted != 1 {
			t.Fatalf("deleted = %d, want 1", stats.Deleted)
		}
		assertNotExist(t, filepath.Join(remote, "obsolete.txt"))
	})

	t.Run("delete extra full sync", func(t *testing.T) {
		local, remote, backend := newBackend(t, nil)
		writeRemote(t, remote, "obsolete.txt", []byte("old"))
		m := mapping(local)
		m.DeleteExtra = true

		if _, err := backend.FullSync(context.Background(), m, nil); err != nil {
			t.Fatalf("full sync: %v", err)
		}
		assertNotExist(t, filepath.Join(remote, "obsolete.txt"))
	})
}

func TestFullSync_UploadsMissingFilesUsingSFTPWalkFallback(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "nested/missing.txt", []byte("new"))

	stats, err := backend.FullSync(context.Background(), mapping(local), nil)
	if err != nil {
		t.Fatalf("full sync: %v", err)
	}
	if stats.Files != 1 {
		t.Fatalf("files = %d, want 1", stats.Files)
	}
	got, err := os.ReadFile(filepath.Join(remote, "nested", "missing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q", got)
	}
}

func TestPushChanges_UploadsSingleFile(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "only.txt", []byte("one"))

	_, err := backend.PushChanges(context.Background(), mapping(local), []syncengine.Change{{
		Op: syncengine.Upload, RelPath: "only.txt",
	}}, nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote, "only.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestPushChanges_ContextCancelMidUploadReturnsError(t *testing.T) {
	local, _, backend := newBackend(t, nil)
	const size = 32 << 20
	file, err := os.Create(filepath.Join(local, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var cancelled atomic.Bool
	_, err = backend.PushChanges(ctx, mapping(local), []syncengine.Change{{
		Op: syncengine.Upload, RelPath: "large.bin", Size: size,
	}}, func(event syncengine.Event) {
		if event.Op == syncengine.Upload && event.Transferred > 0 && cancelled.CompareAndSwap(false, true) {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestPushChanges_ConcurrentUploads(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	changes := make([]syncengine.Change, 24)
	for i := range changes {
		name := fmt.Sprintf("files/%02d.txt", i)
		writeLocal(t, local, name, []byte(name))
		changes[i] = syncengine.Change{Op: syncengine.Upload, RelPath: name}
	}

	stats, err := backend.PushChanges(context.Background(), mapping(local), changes, nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if stats.Files != len(changes) {
		t.Fatalf("files = %d, want %d", stats.Files, len(changes))
	}
	for _, change := range changes {
		if _, err := os.Stat(filepath.Join(remote, filepath.FromSlash(change.RelPath))); err != nil {
			t.Fatalf("%s: %v", change.RelPath, err)
		}
	}
}

func TestFullSync_UsesFindPrintfInventory(t *testing.T) {
	local := t.TempDir()
	writeLocal(t, local, "from-find.txt", []byte("data"))
	info, err := os.Stat(filepath.Join(local, "from-find.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	exec := func(_ string, _ io.Reader, stdout, _ io.Writer) error {
		calls.Add(1)
		_, err := fmt.Fprintf(stdout, "f\tfrom-find.txt\t%d\t%d.0\n", info.Size(), info.ModTime().Unix())
		return err
	}
	_, remote, backend := newBackendWithLocal(t, local, exec)

	stats, err := backend.FullSync(context.Background(), mapping(local), nil)
	if err != nil {
		t.Fatalf("full sync: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("exec calls = %d, want 1", calls.Load())
	}
	if stats.Files != 0 {
		t.Fatalf("files = %d, want 0; find inventory should match local", stats.Files)
	}
	assertNotExist(t, filepath.Join(remote, "from-find.txt"))
}

func newBackend(t *testing.T, exec func(string, io.Reader, io.Writer, io.Writer) error) (string, string, *sftpbackend.Backend) {
	t.Helper()
	return newBackendWithLocal(t, t.TempDir(), exec)
}

func newBackendWithLocal(t *testing.T, local string, exec func(string, io.Reader, io.Writer, io.Writer) error) (string, string, *sftpbackend.Backend) {
	t.Helper()
	remote := t.TempDir()
	srv, err := testsshd.Start(testsshd.Config{
		Root: remote, User: "u", Password: "p", Exec: exec,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	host, portText, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sshclient.Dial(context.Background(), config.Server{
		ID: "test", Host: host, Port: port, User: "u",
		AuthType: "password", Password: "p", HostKeyPolicy: "accept-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	backend, err := sftpbackend.New(client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	if backend.Name() != "sftp" {
		t.Fatalf("name = %q, want sftp", backend.Name())
	}
	return local, remote, backend
}

func mapping(local string) *config.SyncMapping {
	return &config.SyncMapping{
		LocalPath: local, RemotePath: ".", CompareMode: "mtime",
		FileMode: 0o644, DirMode: 0o755,
	}
}

func writeLocal(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRemote(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	writeLocal(t, root, rel, data)
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s still exists (err=%v)", path, err)
	}
}

func TestSyncFiles_UploadsGrayFileAndKeepsRemoteExtra(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "skip.txt", []byte("forced"))
	writeRemote(t, remote, "other.txt", []byte("stay"))
	m := mapping(local)
	m.Excludes = []string{"skip.txt"}
	m.DeleteExtra = true

	stats, err := backend.SyncFiles(context.Background(), m, []string{"skip.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.Deleted != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	got, err := os.ReadFile(filepath.Join(remote, "skip.txt"))
	if err != nil || string(got) != "forced" {
		t.Fatalf("upload = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(remote, "other.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncDirectory_SkipsGrayAndKeepsRemoteExtra(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "dir/keep.txt", []byte("keep"))
	writeLocal(t, local, "dir/skip.txt", []byte("no"))
	writeRemote(t, remote, "dir/old.txt", []byte("old"))
	m := mapping(local)
	m.Excludes = []string{"skip.txt"}
	m.DeleteExtra = true

	stats, err := backend.SyncDirectory(context.Background(), m, "dir", nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Deleted != 0 {
		t.Fatalf("deleted = %d", stats.Deleted)
	}
	if _, err := os.Stat(filepath.Join(remote, "dir", "keep.txt")); err != nil {
		t.Fatal(err)
	}
	assertNotExist(t, filepath.Join(remote, "dir", "skip.txt"))
	if _, err := os.Stat(filepath.Join(remote, "dir", "old.txt")); err != nil {
		t.Fatal("remote extra was deleted")
	}
	if _, err := backend.SyncDirectory(context.Background(), m, "missing", nil); err == nil {
		t.Fatal("missing directory must fail")
	}
}

func TestSyncFiles_RejectsDirectoryBeforeUpload(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "a.txt", []byte("a"))
	if err := os.MkdirAll(filepath.Join(local, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRemote(t, remote, "stay.txt", []byte("stay"))

	_, err := backend.SyncFiles(context.Background(), mapping(local), []string{"a.txt", "sub"}, nil)
	if err == nil {
		t.Fatal("a directory in the file list must be rejected")
	}
	assertNotExist(t, filepath.Join(remote, "a.txt"))
	if _, err := os.Stat(filepath.Join(remote, "stay.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFiles_UploadsGrayFileUnderExcludedParent(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "build/forced.txt", []byte("forced"))
	writeLocal(t, local, "build/other.txt", []byte("other"))
	m := mapping(local)
	m.Excludes = []string{"build/"}

	if _, err := backend.SyncFiles(context.Background(), m, []string{"keep.txt", "build/forced.txt"}, nil); err == nil {
		t.Fatal("missing keep.txt must fail before any upload")
	}
	assertNotExist(t, filepath.Join(remote, "build", "forced.txt"))

	writeLocal(t, local, "plain.txt", []byte("plain"))
	stats, err := backend.SyncFiles(context.Background(), m, []string{"build/forced.txt", "plain.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 || stats.Deleted != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	got, err := os.ReadFile(filepath.Join(remote, "build", "forced.txt"))
	if err != nil || string(got) != "forced" {
		t.Fatalf("forced = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(remote, "plain.txt")); err != nil {
		t.Fatal(err)
	}
	assertNotExist(t, filepath.Join(remote, "build", "other.txt"))
}

func TestSyncDirectory_RejectsExcludedDirectory(t *testing.T) {
	local, _, backend := newBackend(t, nil)
	if err := os.MkdirAll(filepath.Join(local, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := mapping(local)
	m.Excludes = []string{"build/"}
	if _, err := backend.SyncDirectory(context.Background(), m, "build", nil); err == nil {
		t.Fatal("excluded directory must be rejected")
	}
}

func TestSyncDirectory_UploadsWhenRemoteSubdirMissing(t *testing.T) {
	local, remote, backend := newBackend(t, nil)
	writeLocal(t, local, "dir/keep.txt", []byte("keep"))
	writeRemote(t, remote, "sibling.txt", []byte("stay"))
	m := mapping(local)
	m.DeleteExtra = true

	if _, err := backend.SyncDirectory(context.Background(), m, "dir", nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(remote, "dir", "keep.txt"))
	if err != nil || string(got) != "keep" {
		t.Fatalf("keep = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(remote, "sibling.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncChanges_UploadsOnlyNonGrayChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	local, remote, backend := newBackend(t, nil)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = local
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	writeLocal(t, local, "dir/changed.txt", []byte("old"))
	writeLocal(t, local, "dir/same.txt", []byte("untouched"))
	git("add", ".")
	git("commit", "-m", "init")
	writeLocal(t, local, "dir/changed.txt", []byte("new"))
	writeLocal(t, local, "dir/skip.txt", []byte("gray"))
	writeRemote(t, remote, "dir/extra.txt", []byte("stay"))
	m := mapping(local)
	m.Excludes = []string{"skip.txt"}
	m.DeleteExtra = true
	matcher, err := syncengine.NewExcludeMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := syncengine.ListGitChanges(local, matcher)
	if err != nil {
		t.Fatal(err)
	}
	files := syncengine.ChangedFilesUnder(entries, "dir")
	if len(files) != 1 || files[0] != "dir/changed.txt" {
		t.Fatalf("changed files = %#v", files)
	}
	stats, err := backend.SyncFiles(context.Background(), m, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Deleted != 0 {
		t.Fatalf("deleted = %d", stats.Deleted)
	}
	got, err := os.ReadFile(filepath.Join(remote, "dir", "changed.txt"))
	if err != nil || string(got) != "new" {
		t.Fatalf("changed = %q, %v", got, err)
	}
	assertNotExist(t, filepath.Join(remote, "dir", "same.txt"))
	assertNotExist(t, filepath.Join(remote, "dir", "skip.txt"))
	if _, err := os.Stat(filepath.Join(remote, "dir", "extra.txt")); err != nil {
		t.Fatal(err)
	}
}
