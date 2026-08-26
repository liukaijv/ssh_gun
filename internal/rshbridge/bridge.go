package rshbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ssh_gun/internal/config"
	"ssh_gun/internal/sshclient"
)

const marker = "--rsh-bridge"

type Config struct {
	Host          string
	Port          int
	User          string
	AuthType      string
	Password      string
	KeyPath       string
	KeyPassphrase string
	HostKeyPolicy string
}

type countingReader struct {
	source io.Reader
	bytes  atomic.Int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	n, err := r.source.Read(buffer)
	r.bytes.Add(int64(n))
	return n, err
}

func IsBridgeMode(args []string) bool {
	for _, arg := range args {
		if arg == marker {
			return true
		}
	}
	return false
}

// Parse reads connection details from the environment and strips the host
// argument that rsync supplies to every remote-shell command.
func Parse(args []string, getenv func(string) string) (Config, string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	markerIndex := -1
	for i, arg := range args {
		if arg == marker {
			markerIndex = i
			break
		}
	}
	if markerIndex < 0 {
		return Config{}, "", errors.New("--rsh-bridge is required")
	}

	cfg, err := configFromEnv(getenv)
	if err != nil {
		return Config{}, "", err
	}
	commandArgs, err := remoteCommandArgs(args[markerIndex+1:])
	if err != nil {
		return Config{}, "", err
	}
	return cfg, strings.Join(commandArgs, " "), nil
}

func configFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		Host:          getenv("SSH_GUN_HOST"),
		User:          getenv("SSH_GUN_USER"),
		AuthType:      getenv("SSH_GUN_AUTH_TYPE"),
		Password:      getenv("SSH_GUN_PASSWORD"),
		KeyPath:       getenv("SSH_GUN_KEY_PATH"),
		KeyPassphrase: getenv("SSH_GUN_KEY_PASSPHRASE"),
		HostKeyPolicy: getenv("SSH_GUN_HOST_KEY_POLICY"),
		Port:          22,
	}
	if cfg.Host == "" {
		return Config{}, errors.New("SSH_GUN_HOST is required")
	}
	if cfg.User == "" {
		return Config{}, errors.New("SSH_GUN_USER is required")
	}
	if value := getenv("SSH_GUN_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("invalid SSH_GUN_PORT %q", value)
		}
		cfg.Port = port
	}
	if cfg.AuthType == "" {
		if cfg.KeyPath != "" {
			cfg.AuthType = "key"
		} else {
			cfg.AuthType = "password"
		}
	}
	if cfg.AuthType == "key" && cfg.KeyPath == "" {
		return Config{}, errors.New("SSH_GUN_KEY_PATH is required for key authentication")
	}
	if cfg.HostKeyPolicy == "" {
		cfg.HostKeyPolicy = "accept-new"
	}
	return cfg, nil
}

func remoteCommandArgs(args []string) ([]string, error) {
	index := 0
	for index < len(args) {
		switch args[index] {
		case "-l", "-p":
			if index+1 >= len(args) {
				return nil, fmt.Errorf("missing value for bridge option %s", args[index])
			}
			index += 2
		case "-4", "-6":
			index++
		case "--":
			index++
			goto host
		default:
			goto host
		}
	}

host:
	if index >= len(args) {
		return nil, errors.New("rsync bridge host argument is required")
	}
	index++ // rsync syntax requires a host even though the bridge ignores it.
	if index >= len(args) {
		return nil, errors.New("rsync remote command is required")
	}
	return args[index:], nil
}

func (c Config) server() config.Server {
	return config.Server{
		Host: c.Host, Port: c.Port, User: c.User,
		AuthType: c.AuthType, Password: c.Password,
		KeyPath: c.KeyPath, KeyPassphrase: c.KeyPassphrase,
		HostKeyPolicy: c.HostKeyPolicy,
	}
}

// Run dials the configured server and bridges the local rsync process to the
// remote rsync server command over an SSH session.
func Run(args []string) error {
	cfg, command, err := Parse(args, os.Getenv)
	if err != nil {
		return err
	}
	client, err := sshclient.Dial(context.Background(), cfg.server())
	if err != nil {
		return fmt.Errorf("dial bridge SSH: %w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open bridge SSH session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}
	inputSource, closeInput, err := bridgeInput()
	if err != nil {
		return fmt.Errorf("prepare bridge input: %w", err)
	}
	defer closeInput()
	if err := session.Start(command); err != nil {
		return fmt.Errorf("start remote rsync: %w", err)
	}

	input := &countingReader{source: inputSource}
	go func() {
		_, _ = io.Copy(stdin, input)
		_ = stdin.Close()
	}()
	var output sync.WaitGroup
	var stdoutBytes, stderrBytes int64
	output.Add(2)
	go func() {
		defer output.Done()
		stdoutBytes, _ = io.Copy(os.Stdout, stdout)
	}()
	go func() {
		defer output.Done()
		stderrBytes, _ = io.Copy(os.Stderr, stderr)
	}()
	waitStarted := time.Now()
	waitErr := session.Wait()
	output.Wait()
	if waitErr != nil {
		executable, _ := os.Executable()
		workingDirectory, _ := os.Getwd()
		return fmt.Errorf(
			"%w (exe=%q cwd=%q argv=%q remote=%q stdin_bytes=%d stdout_bytes=%d stderr_bytes=%d wait=%s)",
			waitErr,
			executable,
			workingDirectory,
			args,
			command,
			input.bytes.Load(),
			stdoutBytes,
			stderrBytes,
			time.Since(waitStarted),
		)
	}
	return waitErr
}
