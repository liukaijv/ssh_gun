package sftpbackend_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
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
