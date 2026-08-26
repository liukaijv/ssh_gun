package rsyncbackend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ssh_gun/internal/config"
	"ssh_gun/internal/rsyncbin"
	"ssh_gun/internal/syncengine"
)

var baseArgs = []string{
	"-rltz",
	"--no-perms",
	"--no-owner",
	"--no-group",
	"--omit-dir-times",
}

type Command struct {
	Path    string
	Args    []string
	Env     []string
	Cleanup []string // temp files removed after Run
}

type Runner interface {
	Run(context.Context, Command) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, command Command) error {
	defer cleanupCommandFiles(command)
	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Env = command.Env
	output, err := process.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		workingDirectory, _ := os.Getwd()
		diagnostic := fmt.Sprintf(
			"rsync diagnostic: path=%q argv=%q cwd=%q PATH-prefix=%q",
			command.Path,
			command.Args,
			workingDirectory,
			filepath.Dir(command.Path),
		)
		if message != "" {
			return fmt.Errorf("%w: %s\n%s", err, message, diagnostic)
		}
		return fmt.Errorf("%w: %s", err, diagnostic)
	}
	return nil
}

func cleanupCommandFiles(command Command) {
	for _, path := range command.Cleanup {
		_ = os.Remove(path)
	}
}

type Option func(*Backend)

func WithRunner(runner Runner) Option {
	return func(backend *Backend) {
		if runner != nil {
			backend.runner = runner
		}
	}
}

type Backend struct {
	server   config.Server
	rsyncExe string
	selfExe  string
	runner   Runner
}

func New(server config.Server, rsyncExe, selfExe string, options ...Option) *Backend {
	backend := &Backend{
		server: server, rsyncExe: rsyncExe, selfExe: selfExe,
		runner: execRunner{},
	}
	for _, option := range options {
		if option != nil {
			option(backend)
		}
	}
	return backend
}

func (b *Backend) Name() string { return "rsync" }

func (b *Backend) BuildCommand(mapping *config.SyncMapping) (Command, error) {
	if mapping == nil {
		return Command{}, errors.New("sync mapping is required")
	}
	if mapping.LocalPath == "" {
		return Command{}, errors.New("local path is required")
	}
	if mapping.RemotePath == "" {
		return Command{}, errors.New("remote path is required")
	}
	if b.rsyncExe == "" {
		return Command{}, errors.New("rsync executable is required")
	}
	if b.selfExe == "" {
		return Command{}, errors.New("ssh_gun executable is required")
	}
	if b.server.Host == "" || b.server.User == "" {
		return Command{}, errors.New("rsync server host and user are required")
	}
	if strings.Contains(b.selfExe, `"`) {
		return Command{}, errors.New("ssh_gun executable path contains a quote")
	}

	args := append([]string(nil), baseArgs...)
	var cleanup []string
	if mergeArg, mergePath, err := writeManualMergeFilter(syncengine.ExcludesForMapping(mapping)); err != nil {
		return Command{}, err
	} else if mergeArg != "" {
		args = append(args, mergeArg)
		cleanup = append(cleanup, mergePath)
	}
	if mapping.UseGitignore {
		args = append(args, "--filter=:- .gitignore")
	}
	if mapping.DeleteExtra {
		args = append(args, "--delete")
	}
	args = append(args, `--rsh="`+filepath.ToSlash(b.selfExe)+`" --rsh-bridge`)

	local := strings.TrimSuffix(rsyncbin.Cygpath(mapping.LocalPath), "/") + "/"
	remote := strings.TrimSuffix(mapping.RemotePath, "/") + "/"
	args = append(args, local, b.server.User+"@ignored:"+remote)

	binDir := filepath.Dir(b.rsyncExe)
	values := map[string]string{
		"PATH":                    binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"SSH_GUN_HOST":            b.server.Host,
		"SSH_GUN_PORT":            strconv.Itoa(defaultPort(b.server.Port)),
		"SSH_GUN_USER":            b.server.User,
		"SSH_GUN_AUTH_TYPE":       b.server.AuthType,
		"SSH_GUN_PASSWORD":        b.server.Password,
		"SSH_GUN_KEY_PATH":        b.server.KeyPath,
		"SSH_GUN_KEY_PASSPHRASE":  b.server.KeyPassphrase,
		"SSH_GUN_HOST_KEY_POLICY": b.server.HostKeyPolicy,
	}
	return Command{
		Path:    b.rsyncExe,
		Args:    args,
		Env:     replaceEnvironment(os.Environ(), values),
		Cleanup: cleanup,
	}, nil
}

// writeManualMergeFilter converts gitignore-style exclude lines into an rsync
// merge filter file. Rules are written in reverse order so rsync first-match
// approximates git last-match-wins within the manual rule set.
func writeManualMergeFilter(excludes []string) (arg string, path string, err error) {
	rules := make([]string, 0, len(excludes))
	for _, line := range excludes {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			rules = append(rules, "+ "+strings.TrimPrefix(line, "!"))
			continue
		}
		rules = append(rules, "- "+line)
	}
	if len(rules) == 0 {
		return "", "", nil
	}
	for i, j := 0, len(rules)-1; i < j; i, j = i+1, j-1 {
		rules[i], rules[j] = rules[j], rules[i]
	}
	file, err := os.CreateTemp("", "ssh_gun_rsync_filter_*.txt")
	if err != nil {
		return "", "", fmt.Errorf("create rsync filter file: %w", err)
	}
	content := strings.Join(rules, "\n") + "\n"
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", "", fmt.Errorf("write rsync filter file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", "", err
	}
	filterPath := rsyncbin.Cygpath(file.Name())
	return "--filter=merge " + filterPath, file.Name(), nil
}

func (b *Backend) FullSync(ctx context.Context, mapping *config.SyncMapping, emit func(syncengine.Event)) (*syncengine.Stats, error) {
	stats := &syncengine.Stats{Started: time.Now()}
	defer func() { stats.Finished = time.Now() }()
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	command, err := b.BuildCommand(mapping)
	if err != nil {
		return stats, err
	}
	defer cleanupCommandFiles(command)
	if err := b.runner.Run(ctx, command); err != nil {
		wrapped := fmt.Errorf("run rsync: %w", err)
		if emit != nil {
			emit(syncengine.Event{Err: wrapped})
		}
		return stats, wrapped
	}
	return stats, nil
}

// PushChanges intentionally falls back to a complete rsync pass. Rsync's own
// delta algorithm still avoids retransmitting unchanged file contents.
func (b *Backend) PushChanges(ctx context.Context, mapping *config.SyncMapping, _ []syncengine.Change, emit func(syncengine.Event)) (*syncengine.Stats, error) {
	return b.FullSync(ctx, mapping, emit)
}

func defaultPort(port int) int {
	if port == 0 {
		return 22
	}
	return port
}

func replaceEnvironment(existing []string, values map[string]string) []string {
	replaced := make(map[string]struct{}, len(values))
	for key := range values {
		replaced[strings.ToUpper(key)] = struct{}{}
	}
	env := make([]string, 0, len(existing)+len(values))
	for _, entry := range existing {
		key, _, found := strings.Cut(entry, "=")
		if _, replace := replaced[strings.ToUpper(key)]; found && replace {
			continue
		}
		env = append(env, entry)
	}
	for _, key := range []string{
		"PATH", "SSH_GUN_HOST", "SSH_GUN_PORT", "SSH_GUN_USER",
		"SSH_GUN_AUTH_TYPE", "SSH_GUN_PASSWORD", "SSH_GUN_KEY_PATH",
		"SSH_GUN_KEY_PASSPHRASE", "SSH_GUN_HOST_KEY_POLICY",
	} {
		env = append(env, key+"="+values[key])
	}
	return env
}

var _ syncengine.Backend = (*Backend)(nil)
