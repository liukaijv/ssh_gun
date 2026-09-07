package procman

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ssh_gun/internal/config"
)

func TestSplitArgs(t *testing.T) {
	got, err := SplitArgs(`proxy --port 8787 --name "my app"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"proxy", "--port", "8787", "--name", "my app"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
	if _, err := SplitArgs(`"unterminated`); err == nil {
		t.Fatal("expected unclosed quote error")
	}
}

func TestManager_StartStop(t *testing.T) {
	var cmdPath string
	var args string
	if runtime.GOOS == "windows" {
		cmdPath = filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe")
		args = "-t 127.0.0.1"
	} else {
		cmdPath = "/bin/sleep"
		args = "60"
	}
	if _, err := os.Stat(cmdPath); err != nil {
		t.Skip("helper binary unavailable:", cmdPath)
	}

	m := NewManager()
	cfg := config.ManagedProcess{
		ID: "p1", Name: "keepalive", Command: cmdPath, Args: args, Enabled: true,
	}
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}
	st := m.Status("p1")
	if !st.Running || st.PID == 0 || st.UptimeSec < 0 {
		t.Fatalf("status = %#v", st)
	}
	if err := m.Stop("p1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.Status("p1").Running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("process still marked running after stop")
}

func TestManager_RejectsDisabled(t *testing.T) {
	m := NewManager()
	cfg := config.ManagedProcess{
		ID: "p2", Name: "disabled", Command: "echo", Enabled: false,
	}
	if err := m.Start(cfg); err == nil {
		t.Fatal("expected disabled error")
	}
}

func TestResolveCommand_UsesWorkDir(t *testing.T) {
	dir := t.TempDir()
	name := "helper.bin"
	if runtime.GOOS == "windows" {
		name = "helper.exe"
	}
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolveCommand(name, dir)
	if got != full {
		t.Fatalf("resolveCommand = %q, want %q", got, full)
	}
	if got := resolveCommand(full, dir); got != full {
		t.Fatalf("abs command changed: %q", got)
	}
	if got := resolveCommand("missing.exe", dir); got != "missing.exe" {
		t.Fatalf("missing should fall back to PATH name, got %q", got)
	}
	if got := resolveCommand(name, ""); got != name {
		t.Fatalf("empty workdir should keep name, got %q", got)
	}
}

func TestManager_StartRelativeCommandWithWorkDir(t *testing.T) {
	dir := t.TempDir()
	var (
		exeName string
		src     string
		args    string
	)
	if runtime.GOOS == "windows" {
		src = filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe")
		exeName = "ping.exe"
		args = "-n 1 127.0.0.1"
	} else {
		src = "/bin/sleep"
		exeName = "sleep"
		args = "60"
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	dst := filepath.Join(dir, exeName)
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	cfg := config.ManagedProcess{
		ID: "p3", Name: "rel", Command: exeName, Args: args, WorkDir: dir, Enabled: true,
	}
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop("p3") }()
	st := m.Status("p3")
	if !st.Running {
		t.Fatalf("status = %#v", st)
	}
}

func keepaliveBinary(t *testing.T) (cmdPath, args string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		cmdPath = filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe")
		args = "-t 127.0.0.1"
	} else {
		cmdPath = "/bin/sleep"
		args = "60"
	}
	if _, err := os.Stat(cmdPath); err != nil {
		t.Skip("helper binary unavailable:", cmdPath)
	}
	return cmdPath, args
}

func TestManager_PersistsAndClearsState(t *testing.T) {
	cmdPath, args := keepaliveBinary(t)
	statePath := filepath.Join(t.TempDir(), "process_runtime.json")
	m := NewManager(WithStatePath(statePath))
	cfg := config.ManagedProcess{
		ID: "persist-1", Name: "keepalive", Command: cmdPath, Args: args, Enabled: true,
	}
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}
	st := m.Status("persist-1")
	if !st.Running {
		t.Fatalf("status = %#v", st)
	}
	entries := m.state.loadAll()
	if e, ok := entries["persist-1"]; !ok || e.PID != st.PID {
		t.Fatalf("state = %#v, status pid=%d", entries, st.PID)
	}
	if err := m.Stop("persist-1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.Status("persist-1").Running {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(m.state.loadAll()) != 0 {
		t.Fatalf("state not cleared: %#v", m.state.loadAll())
	}
}

func TestManager_RecoverAdoptsLiveProcess(t *testing.T) {
	cmdPath, args := keepaliveBinary(t)
	statePath := filepath.Join(t.TempDir(), "process_runtime.json")
	m1 := NewManager(WithStatePath(statePath))
	cfg := config.ManagedProcess{
		ID: "adopt-1", Name: "keepalive", Command: cmdPath, Args: args, Enabled: true, AutoStart: true,
	}
	if err := m1.Start(cfg); err != nil {
		t.Fatal(err)
	}
	pid := m1.Status("adopt-1").PID
	if pid == 0 {
		t.Fatal("expected pid")
	}

	// Simulate app restart: new manager, old process still alive, in-memory map empty.
	m2 := NewManager(WithStatePath(statePath))
	m2.RecoverOnStartup([]config.ManagedProcess{cfg})
	st := m2.Status("adopt-1")
	if !st.Running || st.PID != pid {
		t.Fatalf("adopted status = %#v, want pid %d", st, pid)
	}
	if err := m2.Start(cfg); err == nil {
		t.Fatal("expected already-running error after adopt")
	}
	if err := m2.Stop("adopt-1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) && !m2.Status("adopt-1").Running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("adopted process still alive after stop")
}

func TestManager_RecoverClearsDeadPID(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "process_runtime.json")
	store := newRuntimeStore(statePath)
	if err := store.save("dead-1", runtimeEntry{
		PID:       1<<30 - 1, // unlikely to exist
		StartedAt: time.Now().Add(-time.Hour),
		Command:   "missing",
	}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(WithStatePath(statePath))
	m.RecoverOnStartup([]config.ManagedProcess{{
		ID: "dead-1", Name: "gone", Command: "missing", Enabled: true,
	}})
	if m.Status("dead-1").Running {
		t.Fatal("dead pid should not be adopted")
	}
	if len(m.state.loadAll()) != 0 {
		t.Fatalf("dead state not cleared: %#v", m.state.loadAll())
	}
}
