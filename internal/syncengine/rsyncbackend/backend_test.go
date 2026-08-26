package rsyncbackend

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

func (r *recordingRunner) Run(_ context.Context, command Command) error {
	r.calls++
	r.last = command
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
