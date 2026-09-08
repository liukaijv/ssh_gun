package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ssh_gun/internal/config"
)

func TestMergeProcessEnv_OverridesAndPreserves(t *testing.T) {
	base := []string{"PATH=/usr/bin", "FOO=old", "BAR=keep"}
	got := mergeProcessEnv(base, map[string]string{"FOO": "new", "BAZ": "added"})
	if envValue(got, "PATH") != "/usr/bin" || envValue(got, "FOO") != "new" ||
		envValue(got, "BAR") != "keep" || envValue(got, "BAZ") != "added" {
		t.Fatalf("got %#v", got)
	}
}

func TestMergeProcessEnv_WindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only case folding")
	}
	base := []string{`Path=C:\Windows`, "Foo=old"}
	got := mergeProcessEnv(base, map[string]string{"PATH": `C:\Custom`, "FOO": "new"})
	if envValue(got, "Path") != `C:\Custom` && envValue(got, "PATH") != `C:\Custom` {
		t.Fatalf("path not overridden: %#v", got)
	}
	if envValue(got, "Foo") != "new" && envValue(got, "FOO") != "new" {
		t.Fatalf("foo not overridden: %#v", got)
	}
}

func TestManager_StartInjectsEnv(t *testing.T) {
	marker := "SSH_GUN_ENV_MARKER"
	want := "https://tokenhub.tencentmaas.com"

	var cmdPath string
	if runtime.GOOS == "windows" {
		cmdPath = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	} else {
		cmdPath = "/bin/sh"
	}
	if _, err := os.Stat(cmdPath); err != nil {
		t.Skip(err)
	}

	merged := mergeProcessEnv(os.Environ(), map[string]string{marker: want})
	if envValue(merged, marker) != want {
		t.Fatalf("merge missing marker: %#v", merged)
	}

	cmd := exec.Command(cmdPath)
	if runtime.GOOS == "windows" {
		cmd.Args = []string{cmdPath, "/C", "echo %" + marker + "%"}
	} else {
		cmd.Args = []string{cmdPath, "-c", `printf %s "$` + marker + `"`}
	}
	cmd.Env = merged
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), want) {
		t.Fatalf("output %q missing %q", out, want)
	}

	m := NewManager()
	keepCmd, keepArgs := keepaliveBinary(t)
	if err := m.Start(config.ManagedProcess{
		ID: "env-keepalive", Name: "keepalive", Command: keepCmd, Args: keepArgs, Enabled: true,
		Env: map[string]string{marker: want},
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop("env-keepalive") }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status("env-keepalive").Running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("keepalive with env did not start")
}

func envValue(env []string, key string) string {
	folded := runtime.GOOS == "windows"
	want := key
	if folded {
		want = strings.ToUpper(key)
	}
	for _, entry := range env {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		got := k
		if folded {
			got = strings.ToUpper(k)
		}
		if got == want {
			return v
		}
	}
	return ""
}
