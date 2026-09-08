// Package procman starts and stops user-defined local background processes.
package procman

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"ssh_gun/internal/config"
)

// Status describes a managed process at query time.
type Status struct {
	Running   bool
	PID       int
	UptimeSec int64
	Err       string
}

// Option configures a Manager.
type Option func(*Manager)

// WithLogger sets the logger used for process lifecycle and output.
func WithLogger(logger *slog.Logger) Option {
	return func(m *Manager) {
		if logger != nil {
			m.log = logger
		}
	}
}

// WithStatePath enables PID persistence at path (JSON). Empty disables persistence.
func WithStatePath(path string) Option {
	return func(m *Manager) {
		m.state = newRuntimeStore(path)
	}
}

// Manager tracks running managed processes by ID.
type Manager struct {
	log   *slog.Logger
	state *runtimeStore

	mu        sync.Mutex
	processes map[string]*runningProcess
}

type runningProcess struct {
	id        string
	name      string
	pid       int
	cmd       *exec.Cmd // nil when adopted after restart
	startedAt time.Time
	err       string
}

func (r *runningProcess) tracked() bool {
	return r != nil && r.pid > 0
}

// NewManager creates an empty process manager.
func NewManager(options ...Option) *Manager {
	m := &Manager{
		log:       slog.New(slog.DiscardHandler),
		processes: make(map[string]*runningProcess),
	}
	for _, option := range options {
		if option != nil {
			option(m)
		}
	}
	return m
}

// Start launches cfg as a background process. ID must match cfg.ID.
func (m *Manager) Start(cfg config.ManagedProcess) error {
	normalized, err := config.NormalizeManagedProcess(cfg)
	if err != nil {
		return err
	}
	if !normalized.Enabled {
		return fmt.Errorf("managed process %q is disabled", normalized.Name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.processes[normalized.ID]; ok && current.tracked() {
		return fmt.Errorf("managed process %q is already running", normalized.Name)
	}

	args, err := SplitArgs(normalized.Args)
	if err != nil {
		return fmt.Errorf("parse args: %w", err)
	}
	command := resolveCommand(normalized.Command, normalized.WorkDir)
	cmd := exec.Command(command, args...)
	if normalized.WorkDir != "" {
		cmd.Dir = normalized.WorkDir
	}
	applyProcessEnv(&cmd.Env, normalized.Env)
	configureSysProcAttr(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %q: %w", normalized.Command, err)
	}

	startedAt := time.Now()
	running := &runningProcess{
		id:        normalized.ID,
		name:      normalized.Name,
		pid:       cmd.Process.Pid,
		cmd:       cmd,
		startedAt: startedAt,
	}
	m.processes[normalized.ID] = running

	if m.state != nil {
		if err := m.state.save(normalized.ID, runtimeEntry{
			PID:       running.pid,
			StartedAt: startedAt,
			Command:   command,
		}); err != nil {
			m.log.Warn("persist process state", "id", normalized.ID, "err", err)
		}
	}

	go pipeOutput(m.log, normalized.Name, "stdout", stdout)
	go pipeOutput(m.log, normalized.Name, "stderr", stderr)
	go m.wait(running)

	m.log.Info("managed process started",
		"id", normalized.ID,
		"name", normalized.Name,
		"pid", running.pid,
		"command", command,
	)
	return nil
}

// Stop terminates a running process and its process tree when possible.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	running, ok := m.processes[id]
	if !ok || !running.tracked() {
		m.mu.Unlock()
		return nil
	}
	pid := running.pid
	name := running.name
	m.mu.Unlock()

	if err := killProcessTree(pid); err != nil {
		// Process may already be gone; still drop tracking.
		if isProcessAlive(pid) {
			return err
		}
	}
	m.mu.Lock()
	if current, ok := m.processes[id]; ok && current.pid == pid {
		delete(m.processes, id)
	}
	m.mu.Unlock()
	if m.state != nil {
		m.state.remove(id)
	}
	m.log.Info("managed process stop requested", "id", id, "name", name, "pid", pid)
	return nil
}

// Status returns the current runtime status for id.
func (m *Manager) Status(id string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	running, ok := m.processes[id]
	if !ok || !running.tracked() {
		return Status{}
	}
	return Status{
		Running:   true,
		PID:       running.pid,
		UptimeSec: int64(time.Since(running.startedAt).Seconds()),
		Err:       running.err,
	}
}

// StopAll stops every tracked process.
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Stop(id)
	}
}

// RecoverOnStartup reloads persisted PIDs, adopts live processes, and cleans dead ones.
// Config entries that no longer exist but still have a live PID are killed.
func (m *Manager) RecoverOnStartup(cfgs []config.ManagedProcess) {
	if m.state == nil {
		return
	}
	known := make(map[string]config.ManagedProcess, len(cfgs))
	for _, cfg := range cfgs {
		known[cfg.ID] = cfg
	}
	entries := m.state.loadAll()
	for id, entry := range entries {
		alive := isProcessAlive(entry.PID)
		cfg, inConfig := known[id]
		if !alive {
			m.state.remove(id)
			continue
		}
		if !inConfig {
			m.log.Info("killing orphan managed process", "id", id, "pid", entry.PID)
			_ = killProcessTree(entry.PID)
			m.state.remove(id)
			continue
		}
		name := cfg.Name
		if name == "" {
			name = id
		}
		m.adoptLocked(id, name, entry)
		m.log.Info("managed process adopted",
			"id", id,
			"name", name,
			"pid", entry.PID,
			"command", entry.Command,
		)
	}
}

func (m *Manager) adoptLocked(id, name string, entry runtimeEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[id] = &runningProcess{
		id:        id,
		name:      name,
		pid:       entry.PID,
		startedAt: entry.StartedAt,
	}
}

func (m *Manager) wait(running *runningProcess) {
	if running.cmd == nil {
		return
	}
	err := running.cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.processes[running.id]
	if !ok || current != running {
		return
	}
	if err != nil {
		running.err = err.Error()
		m.log.Error("managed process exited", "id", running.id, "name", running.name, "err", err)
	} else {
		m.log.Info("managed process exited", "id", running.id, "name", running.name)
	}
	delete(m.processes, running.id)
	if m.state != nil {
		m.state.remove(running.id)
	}
}

func pipeOutput(log *slog.Logger, name, stream string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		log.Info("managed process output", "name", name, "stream", stream, "line", line)
	}
}

// SplitArgs splits a single-line argument string using shell-style quoting.
// Supports double and single quotes; backslash escapes the next character.
func SplitArgs(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var (
		args   []string
		cur    strings.Builder
		quote  rune
		escape bool
	)
	flush := func() {
		args = append(args, cur.String())
		cur.Reset()
	}
	for _, r := range s {
		if escape {
			cur.WriteRune(r)
			escape = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escape = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		switch {
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				flush()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if escape {
		return nil, fmt.Errorf("trailing backslash")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote")
	}
	if cur.Len() > 0 {
		flush()
	}
	return args, nil
}

// resolveCommand returns an absolute path when command is relative and exists
// under workDir. Absolute commands and PATH lookups are left unchanged.
func resolveCommand(command, workDir string) string {
	if command == "" || filepath.IsAbs(command) || workDir == "" {
		return command
	}
	candidate := filepath.Join(workDir, command)
	st, err := os.Stat(candidate)
	if err != nil || st.IsDir() {
		return command
	}
	return candidate
}
