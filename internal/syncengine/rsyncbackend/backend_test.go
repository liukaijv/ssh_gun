package rsyncbackend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"ssh_gun/internal/config"
	"ssh_gun/internal/syncengine"
)

func TestBuildCommandUsesCwRsyncPathsBridgeAndMappingOptions(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "cwrsync", "bin")
	rsyncExe := filepath.Join(binDir, "rsync.exe")
	selfExe := filepath.Join(t.TempDir(), "Program Files", "ssh_gun.exe")
	server := config.Server{
		Host: "real.example", Port: 2222, User: "alice",
		AuthType: "password", Password: "secret", HostKeyPolicy: "accept-new",
	}
	backend := New(server, rsyncExe, selfExe)
	mapping := &config.SyncMapping{
		LocalPath:   `D:\project`,
		RemotePath:  "/opt/project",
		Excludes:    []string{".git/", "*.tmp"},
		DeleteExtra: true,
	}

	command, err := backend.BuildCommand(mapping)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCommandFiles(command) })
	if command.Path != rsyncExe {
		t.Fatalf("path = %q, want %q", command.Path, rsyncExe)
	}
	if !hasArgPrefix(command.Args, "--filter=merge ") {
		t.Fatalf("missing merge filter: %#v", command.Args)
	}
	if !slices.Contains(command.Args, "--delete") {
		t.Fatalf("missing --delete: %#v", command.Args)
	}
	if !slices.Contains(command.Args, "--info=name1,del1") {
		t.Fatalf("missing --info=name1,del1: %#v", command.Args)
	}
	if !slices.Contains(command.Args, "--outbuf=L") {
		t.Fatalf("missing --outbuf=L: %#v", command.Args)
	}
	if !slices.Contains(command.Args, `--rsh="`+filepath.ToSlash(selfExe)+`" --rsh-bridge`) {
		t.Fatalf("missing rsh bridge: %#v", command.Args)
	}
	if !slices.Contains(command.Args, "/cygdrive/d/project/") {
		t.Fatalf("missing local path: %#v", command.Args)
	}
	if !slices.Contains(command.Args, "alice@ignored:/opt/project/") {
		t.Fatalf("missing remote path: %#v", command.Args)
	}
	if len(command.Cleanup) != 1 {
		t.Fatalf("cleanup = %#v, want one temp filter file", command.Cleanup)
	}
	if _, err := os.Stat(command.Cleanup[0]); err != nil {
		t.Fatalf("filter file missing: %v", err)
	}
	body, err := os.ReadFile(command.Cleanup[0])
	if err != nil {
		t.Fatal(err)
	}
	// Reversed: *.tmp then .git/
	if got := string(body); !strings.Contains(got, "- *.tmp") || !strings.Contains(got, "- .git/") {
		t.Fatalf("filter body = %q", got)
	}
	if got := envValue(command.Env, "SSH_GUN_HOST"); got != "real.example" {
		t.Fatalf("SSH_GUN_HOST = %q", got)
	}
	if got := envValue(command.Env, "SSH_GUN_PORT"); got != "2222" {
		t.Fatalf("SSH_GUN_PORT = %q", got)
	}
	if got := envValue(command.Env, "SSH_GUN_PASSWORD"); got != "secret" {
		t.Fatalf("SSH_GUN_PASSWORD = %q", got)
	}
	pathValue := envValue(command.Env, "PATH")
	if !strings.HasPrefix(strings.ToLower(pathValue), strings.ToLower(binDir+string(filepath.ListSeparator))) {
		t.Fatalf("PATH does not start with cwrsync bin: %q", pathValue)
	}
}

func TestBuildCommandAddsGitignoreFilter(t *testing.T) {
	backend := New(
		config.Server{Host: "example.test", User: "alice"},
		filepath.Join(t.TempDir(), "bin", "rsync.exe"),
		filepath.Join(t.TempDir(), "ssh_gun.exe"),
	)
	mapping := &config.SyncMapping{
		LocalPath:    `C:\project`,
		RemotePath:   "/srv/project",
		Excludes:     []string{".git/", "!keep.txt"},
		UseGitignore: true,
	}
	command, err := backend.BuildCommand(mapping)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCommandFiles(command) })
	if !hasArgPrefix(command.Args, "--filter=merge ") {
		t.Fatalf("missing merge filter: %#v", command.Args)
	}
	if !slices.Contains(command.Args, "--filter=:- .gitignore") {
		t.Fatalf("missing gitignore filter: %#v", command.Args)
	}
	body, err := os.ReadFile(command.Cleanup[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, "+ keep.txt") || !strings.Contains(got, "- .git/") {
		t.Fatalf("filter body = %q", got)
	}
}

func TestBuildCommandIncludeModeFilter(t *testing.T) {
	backend := New(
		config.Server{Host: "example.test", User: "alice"},
		filepath.Join(t.TempDir(), "bin", "rsync.exe"),
		filepath.Join(t.TempDir(), "ssh_gun.exe"),
	)
	mapping := &config.SyncMapping{
		LocalPath:  `C:\project`,
		RemotePath: "/srv/project",
		FilterMode: config.FilterModeInclude,
		Includes:   []string{"src/", "docs/guide.md"},
		Excludes:   []string{"src/skip.go"},
	}
	command, err := backend.BuildCommand(mapping)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCommandFiles(command) })
	body, err := os.ReadFile(command.Cleanup[0])
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "- .git/") || !strings.Contains(got, "+ /src/") || !strings.Contains(got, "+ /src/***") {
		t.Fatalf("include dir rules missing: %q", got)
	}
	if !strings.Contains(got, "+ /docs/") || !strings.Contains(got, "+ /docs/guide.md") {
		t.Fatalf("include file parent rules missing: %q", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "- *") {
		t.Fatalf("include filter should end with exclude-all: %q", got)
	}
	if strings.Contains(got, "skip.go") {
		t.Fatalf("exclude text must not apply in include mode: %q", got)
	}
}

func TestPushChangesFallsBackToFullSync(t *testing.T) {
	runner := &recordingRunner{}
	backend := New(
		config.Server{Host: "example.test", Port: 22, User: "alice", Password: "p"},
		filepath.Join(t.TempDir(), "bin", "rsync.exe"),
		filepath.Join(t.TempDir(), "ssh_gun.exe"),
		WithRunner(runner),
	)
	mapping := &config.SyncMapping{LocalPath: `C:\project`, RemotePath: "/srv/project"}

	stats, err := backend.PushChanges(context.Background(), mapping, []syncengine.Change{{Op: syncengine.Upload, RelPath: "file.txt"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if stats.Started.IsZero() || stats.Finished.IsZero() {
		t.Fatalf("stats timestamps not populated: %+v", stats)
	}
}

func hasArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

type recordingRunner struct {
	calls int
	last  Command
}

func (r *recordingRunner) Run(_ context.Context, command Command, _ func(string)) error {
	r.calls++
	r.last = command
	return nil
}

func TestParseRsyncProgressLine(t *testing.T) {
	tests := []struct {
		line string
		op   syncengine.ChangeOp
		rel  string
		ok   bool
	}{
		{line: "src/a.txt", op: syncengine.Upload, rel: "src/a.txt", ok: true},
		{line: "deleting old.txt", op: syncengine.Delete, rel: "old.txt", ok: true},
		{line: "deleting directory tmp", op: syncengine.Delete, rel: "tmp", ok: true},
		{line: "rsync: connection unexpectedly closed", ok: false},
		{line: "sent 123 bytes  received 45 bytes  1.00 bytes/sec", ok: false},
		{line: "total size is 999", ok: false},
		{line: "", ok: false},
		{line: "   ", ok: false},
	}
	for _, tt := range tests {
		op, rel, ok := parseRsyncProgressLine(tt.line)
		if ok != tt.ok || op != tt.op || rel != tt.rel {
			t.Errorf("parseRsyncProgressLine(%q) = %q %q %v, want %q %q %v", tt.line, op, rel, ok, tt.op, tt.rel, tt.ok)
		}
	}
}

func TestFullSyncCountsRsyncOutput(t *testing.T) {
	runner := &scriptedRunner{lines: []string{
		"a.txt",
		"deleting old.txt",
		"rsync: skipped",
		"b.txt",
	}}
	backend := New(
		config.Server{Host: "example.test", Port: 22, User: "alice", Password: "p"},
		filepath.Join(t.TempDir(), "bin", "rsync.exe"),
		filepath.Join(t.TempDir(), "ssh_gun.exe"),
		WithRunner(runner),
	)
	mapping := &config.SyncMapping{Name: "docs", LocalPath: `C:\project`, RemotePath: "/srv/project"}
	stats, err := backend.FullSync(context.Background(), mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 || stats.Deleted != 1 {
		t.Fatalf("stats = %+v, want files=2 deleted=1", stats)
	}
}

func TestExecRunnerStreamsStdoutBeforeExit(t *testing.T) {
	if os.Getenv("SSH_GUN_RSYNC_HELPER") == "1" {
		fmt.Println("hello.txt")
		_ = os.Stdout.Sync()
		release := os.Getenv("RELEASE_FILE")
		for {
			if _, err := os.Stat(release); err == nil {
				os.Exit(0)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	release := filepath.Join(t.TempDir(), "release")
	gotLine := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		done <- execRunner{}.Run(context.Background(), Command{
			Path: os.Args[0],
			Args: []string{"-test.run=^TestExecRunnerStreamsStdoutBeforeExit$"},
			Env: append(os.Environ(),
				"SSH_GUN_RSYNC_HELPER=1",
				"RELEASE_FILE="+release,
			),
		}, func(line string) {
			gotLine <- line
		})
	}()

	deadline := time.After(15 * time.Second)
	for {
		select {
		case line := <-gotLine:
			if line != "hello.txt" {
				continue
			}
			select {
			case err := <-done:
				t.Fatalf("process exited before release: %v", err)
			default:
			}
			if err := os.WriteFile(release, []byte("1"), 0o644); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				return
			case <-time.After(15 * time.Second):
				t.Fatal("process did not exit after release")
			}
		case err := <-done:
			t.Fatalf("process exited before stdout callback: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for streamed stdout")
		}
	}
}

type scriptedRunner struct {
	lines []string
}

func (r *scriptedRunner) Run(_ context.Context, _ Command, onLine func(string)) error {
	for _, line := range r.lines {
		if onLine != nil {
			onLine(line)
		}
	}
	return nil
}

func envValue(env []string, key string) string {
	prefix := strings.ToUpper(key) + "="
	for _, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), prefix) {
			return entry[len(prefix):]
		}
	}
	return ""
}
