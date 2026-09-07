package config

import (
	"fmt"
	"strings"
	"time"
)

const (
	CurrentVersion = 1
	SecretMask     = "********"
)

type File struct {
	Version      int              `toml:"version"`
	SyncBackend  string           `toml:"sync_backend"`
	Servers      []Server         `toml:"server"`
	SyncMappings []SyncMapping    `toml:"sync_mapping"`
	PortForwards []PortForward    `toml:"port_forward"`
	Processes    []ManagedProcess `toml:"process"`
	UI           UIState          `toml:"ui"`
}

const (
	SyncBackendSFTP  = "sftp"
	SyncBackendRsync = "rsync"
)

func NormalizeSyncBackend(backend string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(backend)); normalized {
	case "":
		return SyncBackendSFTP, nil
	case SyncBackendSFTP, SyncBackendRsync:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid sync backend %q", backend)
	}
}

type Server struct {
	ID            string `toml:"id"`
	Name          string `toml:"name"`
	Host          string `toml:"host"`
	Port          int    `toml:"port"`
	User          string `toml:"user"`
	AuthType      string `toml:"auth_type"` // password | key
	Password      string `toml:"password,omitempty"`
	KeyPath       string `toml:"key_path,omitempty"`
	KeyPassphrase string `toml:"key_passphrase,omitempty"`
	HostKeyPolicy string `toml:"host_key_policy"` // accept-new | strict
}

// NormalizeServer trims required string fields and validates name/host/user/port.
func NormalizeServer(srv Server) (Server, error) {
	srv.Name = strings.TrimSpace(srv.Name)
	srv.Host = strings.TrimSpace(srv.Host)
	srv.User = strings.TrimSpace(srv.User)
	if srv.Name == "" {
		return Server{}, fmt.Errorf("server name is required")
	}
	if srv.Host == "" {
		return Server{}, fmt.Errorf("server host is required")
	}
	if srv.User == "" {
		return Server{}, fmt.Errorf("server user is required")
	}
	if srv.Port < 1 || srv.Port > 65535 {
		return Server{}, fmt.Errorf("server port must be between 1 and 65535")
	}
	if srv.HostKeyPolicy == "" {
		srv.HostKeyPolicy = "accept-new"
	}
	return srv, nil
}

// ServerUI is a server view safe for the frontend (no plaintext secrets).
type ServerUI struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	AuthType      string `json:"authType"`
	KeyPath       string `json:"keyPath"`
	HostKeyPolicy string `json:"hostKeyPolicy"`
	HasPassword   bool   `json:"hasPassword"`
	HasPassphrase bool   `json:"hasPassphrase"`
	Password      string `json:"password,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

type SyncMapping struct {
	ID           string   `toml:"id"`
	ServerID     string   `toml:"server_id"`
	Name         string   `toml:"name"`
	LocalPath    string   `toml:"local_path"`
	RemotePath   string   `toml:"remote_path"`
	Excludes     []string `toml:"excludes"`
	UseGitignore bool     `toml:"use_gitignore"`
	DeleteExtra  bool     `toml:"delete_extra"`
	Backend      string   `toml:"backend"`
	CompareMode  string   `toml:"compare_mode"`
	FileMode     uint32   `toml:"file_mode"`
	DirMode      uint32   `toml:"dir_mode"`
	AutoSync     bool     `toml:"auto_sync"`
	DebounceMs   int      `toml:"debounce_ms"`
	Enabled      bool     `toml:"enabled"`

	LastSyncAt     time.Time `toml:"last_sync_at,omitempty"`
	LastSyncResult string    `toml:"last_sync_result,omitempty"`
	LastSyncError  string    `toml:"last_sync_error,omitempty"`
}

// NormalizeSyncMapping trims required fields and validates name/server/local/remote.
func NormalizeSyncMapping(m SyncMapping) (SyncMapping, error) {
	m.ServerID = strings.TrimSpace(m.ServerID)
	m.Name = strings.TrimSpace(m.Name)
	m.LocalPath = strings.TrimSpace(m.LocalPath)
	m.RemotePath = strings.TrimSpace(m.RemotePath)
	if m.Name == "" {
		return SyncMapping{}, fmt.Errorf("sync mapping name is required")
	}
	if m.ServerID == "" {
		return SyncMapping{}, fmt.Errorf("sync mapping server is required")
	}
	if m.LocalPath == "" {
		return SyncMapping{}, fmt.Errorf("sync mapping local path is required")
	}
	if m.RemotePath == "" {
		return SyncMapping{}, fmt.Errorf("sync mapping remote path is required")
	}
	if m.Backend == "" {
		m.Backend = "sftp"
	}
	if m.DebounceMs == 0 {
		m.DebounceMs = 500
	}
	if m.FileMode == 0 {
		m.FileMode = 0o644
	}
	if m.DirMode == 0 {
		m.DirMode = 0o755
	}
	return m, nil
}

// Port forward types (empty Type means local for backward compatibility).
const (
	ForwardTypeLocal   = "local"
	ForwardTypeRemote  = "remote"
	ForwardTypeDynamic = "dynamic"
)

type PortForward struct {
	ID         string `toml:"id"`
	ServerID   string `toml:"server_id"`
	Name       string `toml:"name"`
	Type       string `toml:"type"` // local | remote | dynamic; empty = local
	LocalAddr  string `toml:"local_addr"`
	LocalPort  int    `toml:"local_port"`
	RemoteHost string `toml:"remote_host"`
	RemotePort int    `toml:"remote_port"`
	AutoStart  bool   `toml:"auto_start"`
}

// NormalizedType returns the effective forward type.
func (f PortForward) NormalizedType() string {
	switch f.Type {
	case "", ForwardTypeLocal:
		return ForwardTypeLocal
	case ForwardTypeRemote, ForwardTypeDynamic:
		return f.Type
	default:
		return f.Type
	}
}

// ManagedProcess is a user-defined local background process.
type ManagedProcess struct {
	ID        string `toml:"id"`
	Name      string `toml:"name"`
	Command   string `toml:"command"` // executable path or name on PATH
	Args      string `toml:"args"`    // single-line args, shell-style split
	WorkDir   string `toml:"work_dir"`
	AutoStart bool   `toml:"auto_start"`
	Enabled   bool   `toml:"enabled"`
}

type UIState struct {
	WindowWidth  int    `toml:"window_width"`
	WindowHeight int    `toml:"window_height"`
	Maximized    bool   `toml:"maximized"`
	Theme        string `toml:"theme"`
	Language     string `toml:"language"`
	LastView     string `toml:"last_view"`
}

func NormalizeTheme(theme string) (string, error) {
	switch theme {
	case "":
		return "system", nil
	case "system", "light", "dark":
		return theme, nil
	default:
		return "", fmt.Errorf("invalid theme %q", theme)
	}
}

func NormalizeLanguage(language string) (string, error) {
	switch language {
	case "":
		return "zh-CN", nil
	case "zh-CN", "en-US":
		return language, nil
	default:
		return "", fmt.Errorf("invalid language %q", language)
	}
}

// InUseError is returned when deleting a server that still has dependents.
type InUseError struct {
	Mappings []string
	Forwards []string
}

func (e *InUseError) Error() string {
	return "server is still referenced by sync mappings or port forwards"
}
