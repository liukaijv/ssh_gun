package rsyncbackend

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	"--info=name1,del1",
	"--outbuf=L",
}

type Command struct {
	Path    string
	Args    []string
	Env     []string
	Cleanup []string // temp files removed after Run
}

type Runner interface {
	Run(ctx context.Context, command Command, onLine func(string)) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, command Command, onLine func(string)) error {
	defer cleanupCommandFiles(command)
	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Env = command.Env
	configureRsyncCmd(process)
	stdout, err := process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("rsync stdout: %w", err)
	}
	stderr, err := process.StderrPipe()
	if err != nil {
		return fmt.Errorf("rsync stderr: %w", err)
	}
	if err := process.Start(); err != nil {
		return err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanRsyncOutput(stdout, &stdoutBuf, onLine)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- process.Wait() }()
	wg.Wait()
	waitErr := <-waitCh
	if waitErr == nil {
		return nil
	}
	message := strings.TrimSpace(stdoutBuf.String() + "\n" + stderrBuf.String())
	workingDirectory, _ := os.Getwd()
	diagnostic := fmt.Sprintf(
		"rsync diagnostic: path=%q argv=%q cwd=%q PATH-prefix=%q",
		command.Path,
		command.Args,
		workingDirectory,
		filepath.Dir(command.Path),
	)
	if message != "" {
		return fmt.Errorf("%w: %s\n%s", waitErr, message, diagnostic)
	}
	return fmt.Errorf("%w: %s", waitErr, diagnostic)
}

func scanRsyncOutput(r io.Reader, buf *bytes.Buffer, onLine func(string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if buf != nil {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
		if onLine != nil {
			onLine(line)
		}
	}
}

func parseRsyncProgressLine(line string) (op syncengine.ChangeOp, rel string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "rsync:") || strings.HasPrefix(lower, "rsync error:") {
		return "", "", false
	}
	if strings.HasPrefix(lower, "sent ") || strings.HasPrefix(lower, "total size is") {
		return "", "", false
	}
	if rest, found := strings.CutPrefix(line, "deleting "); found {
		rest = strings.TrimPrefix(rest, "directory ")
		return syncengine.Delete, rest, true
	}
	return syncengine.Upload, line, true
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
	if mergeArg, mergePath, err := writeMappingMergeFilter(mapping); err != nil {
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

func writeMappingMergeFilter(mapping *config.SyncMapping) (arg string, path string, err error) {
	mode := config.FilterModeExclude
	if mapping != nil {
		normalized, normErr := config.NormalizeFilterMode(mapping.FilterMode)
		if normErr != nil {
			return "", "", normErr
		}
		mode = normalized
	}
	if mode == config.FilterModeInclude {
		includes := []string(nil)
		if mapping != nil {
			includes = mapping.Includes
		}
		return writeFilterFile(includeMergeRules(syncengine.DefaultExcludes(), includes))
	}
	return writeManualMergeFilter(syncengine.ExcludesForMapping(mapping))
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
	return writeFilterFile(rules)
}

func includeMergeRules(defaults, includes []string) []string {
	rules := make([]string, 0, len(defaults)+len(includes)*4+2)
	for _, line := range defaults {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, "- "+line)
	}
	allDirs := false
	for _, line := range includes {
		expanded, needDirs := expandIncludeRsync(line)
		if needDirs {
			allDirs = true
		}
		rules = append(rules, expanded...)
	}
	if allDirs {
		rules = append(rules, "+ */")
	}
	rules = append(rules, "- *")
	return rules
}

func expandIncludeRsync(line string) (rules []string, allDirs bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, false
	}
	if strings.HasPrefix(line, "!") {
		return []string{"- " + strings.TrimPrefix(line, "!")}, false
	}
	pat := strings.TrimPrefix(line, "/")
	if strings.ContainsAny(pat, "*?[") && !strings.Contains(pat, "/") {
		return []string{"+ " + pat}, true
	}
	trimmed := strings.Trim(pat, "/")
	if trimmed == "" {
		return []string{"+ *"}, false
	}
	parts := strings.Split(trimmed, "/")
	acc := ""
	for _, part := range parts[:len(parts)-1] {
		acc += "/" + part
		rules = append(rules, "+ "+acc+"/")
	}
	rules = append(rules, "+ /"+trimmed)
	if strings.HasSuffix(pat, "/") || !strings.ContainsAny(parts[len(parts)-1], "*?[") {
		rules = append(rules, "+ /"+trimmed+"/")
		rules = append(rules, "+ /"+trimmed+"/***")
	}
	return rules, false
}

func writeFilterFile(rules []string) (arg string, path string, err error) {
	if len(rules) == 0 {
		return "", "", nil
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
	progress := syncengine.NewProgressLog(syncengine.MappingLogName(mapping))
	progress.Begin("transfer")
	stopHeartbeat := progress.StartHeartbeat(ctx)
	defer stopHeartbeat()
	if err := b.runner.Run(ctx, command, func(line string) {
		op, rel, ok := parseRsyncProgressLine(line)
		if !ok {
			return
		}
		if op == syncengine.Delete {
			stats.Deleted++
		} else {
			stats.Files++
		}
		progress.Record(op, rel, 0)
		if emit != nil {
			emit(syncengine.Event{Op: op, RelPath: rel})
		}
	}); err != nil {
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
