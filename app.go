package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh_gun/internal/applog"
	"ssh_gun/internal/config"
	"ssh_gun/internal/forward"
	"ssh_gun/internal/rsyncbin"
	"ssh_gun/internal/shellopen"
	"ssh_gun/internal/sshclient"
	"ssh_gun/internal/syncengine"
	"ssh_gun/internal/syncengine/rsyncbackend"
	"ssh_gun/internal/syncengine/sftpbackend"
)

// App is the Wails binding layer — keep business logic in internal/ packages.
type App struct {
	ctx    context.Context
	store  *config.Store
	pool   *sshclient.Pool
	fwd    *forward.Manager
	engine *syncengine.Engine
	log    *slog.Logger
	ring   *applog.RingBuffer

	// quitting separates a real quit (tray menu) from closing the window,
	// which only hides it into the notification area.
	quitting atomic.Bool
}

func NewApp() *App {
	return &App{pool: sshclient.NewPool()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	logDir := filepath.Join(userLocalAppData(), "ssh_gun", "logs")
	logger, ring, err := applog.New(logDir, slog.LevelInfo)
	if err != nil {
		logger = slog.Default()
		ring = applog.NewRingBuffer(1000)
	}
	a.log = logger
	a.ring = ring
	slog.SetDefault(logger)

	path := defaultConfigPath()
	store, err := config.Open(path)
	if err != nil {
		a.log.Error("open config", "err", err)
		store, _ = config.Open(path)
	}
	a.store = store
	a.fwd = forward.NewManager(a.pool, forward.WithLogger(a.log))
	syncBackend := a.store.SyncBackend()
	a.engine = syncengine.NewEngineWithFactory(
		func(id string) (config.Server, error) { return a.store.GetServer(id) },
		func(ctx context.Context, srv config.Server, mapping *config.SyncMapping) (syncengine.Backend, error) {
			switch syncBackend {
			case config.SyncBackendSFTP:
				client, err := a.pool.Get(ctx, srv)
				if err != nil {
					return nil, err
				}
				return sftpbackend.New(client)
			case config.SyncBackendRsync:
				runtimeDir, err := rsyncbin.Extract(
					cwrsyncAssets,
					"assets/cwrsync_6.4.8_x64",
					userLocalAppData(),
				)
				if err != nil {
					return nil, err
				}
				selfExe, err := os.Executable()
				if err != nil {
					return nil, fmt.Errorf("locate ssh_gun executable: %w", err)
				}
				return rsyncbackend.New(
					srv,
					filepath.Join(runtimeDir, "bin", "rsync.exe"),
					selfExe,
				), nil
			default:
				return nil, fmt.Errorf("unknown sync backend %q", syncBackend)
			}
		},
		syncengine.WithEmitter(func(ev syncengine.Event) {
			payload := map[string]any{
				"op":          string(ev.Op),
				"relPath":     ev.RelPath,
				"transferred": ev.Transferred,
				"total":       ev.Total,
			}
			if ev.Err != nil {
				payload["error"] = ev.Err.Error()
				a.log.Error("sync file", "path", ev.RelPath, "err", ev.Err)
			} else {
				a.log.Debug("sync file", "path", ev.RelPath, "op", ev.Op, "file_sync", true)
			}
			runtime.EventsEmit(a.ctx, "sync:event", payload)
		}),
	)

	a.startAutostart()

	go startTray(a)
}

// beforeClose turns the window close button into "hide to tray" so background
// sync and port forwards keep running. Only requestQuit lets the app exit.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	runtime.WindowHide(ctx)
	a.log.Info("window hidden to tray")
	return true
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

func (a *App) requestQuit() {
	if a.ctx == nil {
		return
	}
	a.quitting.Store(true)
	runtime.Quit(a.ctx)
}

func (a *App) stopAllRuntime() {
	if a.engine != nil && a.store != nil {
		for _, m := range a.store.ListSyncMappings() {
			_ = a.engine.StopMapping(m.ID)
		}
	}
	if a.fwd != nil && a.store != nil {
		for _, f := range a.store.ListPortForwards() {
			_ = a.fwd.Stop(f.ID)
		}
	}
}

func (a *App) startAutostart() {
	if a.store == nil {
		return
	}
	for _, f := range a.store.ListPortForwards() {
		if !f.AutoStart {
			continue
		}
		srv, err := a.store.GetServer(f.ServerID)
		if err != nil {
			continue
		}
		_ = a.fwd.Start(a.ctx, f, srv)
	}
	for _, m := range a.store.ListSyncMappings() {
		if m.AutoSync && m.Enabled {
			mm := m
			_ = a.engine.StartMapping(&mm)
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.stopAllRuntime()
	if a.pool != nil {
		a.pool.Close()
	}
	stopTray()
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "ssh_gun", "config.toml")
}

func openConfigDirectory(configPath string, opener func(string) error) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := opener(dir); err != nil {
		return fmt.Errorf("open config directory: %w", err)
	}
	return nil
}

func userLocalAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "."
	}
	return dir
}

// --- Servers ---

func (a *App) ListServers() []config.ServerUI {
	return a.store.ListServersForUI()
}

func (a *App) UpsertServer(srv config.Server) error {
	return a.store.UpsertServer(srv)
}

func (a *App) DeleteServer(id string) error {
	return a.store.DeleteServer(id)
}

func (a *App) TestServerConnection(id string) (map[string]any, error) {
	srv, err := a.store.GetServer(id)
	if err != nil {
		return nil, err
	}
	ok, msg, err := sshclient.TestConnection(a.ctx, srv)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": ok, "message": msg}, nil
}

// --- Sync mappings ---

func (a *App) SelectLocalDirectory(defaultPath string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "选择本地目录",
		DefaultDirectory: defaultPath,
	})
}

func (a *App) ListSyncMappings() []config.SyncMapping {
	return a.store.ListSyncMappings()
}

func (a *App) UpsertSyncMapping(m config.SyncMapping) error {
	if err := a.store.UpsertSyncMapping(m); err != nil {
		return err
	}
	_ = a.engine.StopMapping(m.ID)
	if m.AutoSync && m.Enabled {
		mm := m
		return a.engine.StartMapping(&mm)
	}
	return nil
}

func (a *App) DeleteSyncMapping(id string) error {
	_ = a.engine.StopMapping(id)
	return a.store.DeleteSyncMapping(id)
}

func (a *App) RunFullSync(id string) (map[string]any, error) {
	m, err := a.store.GetSyncMapping(id)
	if err != nil {
		return nil, err
	}
	if !m.Enabled {
		return nil, fmt.Errorf("sync mapping is disabled")
	}
	a.log.Info("sync start", "mapping", m.Name, "trigger", "manual")
	start := time.Now()
	stats, err := a.engine.FullSync(a.ctx, &m)
	result := "ok"
	errMsg := ""
	if err != nil {
		if errors.Is(err, syncengine.ErrSyncInProgress) {
			result = "busy"
			errMsg = err.Error()
		} else {
			result = "failed"
			errMsg = err.Error()
			a.log.Error("sync failed", "mapping", m.Name, "err", err)
		}
	} else {
		a.log.Info("sync end",
			"mapping", m.Name,
			"files", stats.Files,
			"deleted", stats.Deleted,
			"bytes", stats.Bytes,
			"dur", time.Since(start).String(),
		)
	}
	m.LastSyncAt = time.Now()
	m.LastSyncResult = result
	m.LastSyncError = errMsg
	_ = a.store.UpsertSyncMapping(m)
	out := map[string]any{"result": result, "error": errMsg}
	if stats != nil {
		out["files"] = stats.Files
		out["deleted"] = stats.Deleted
		out["bytes"] = stats.Bytes
	}
	return out, nil
}

// --- Port forwards ---

func (a *App) ListPortForwards() []config.PortForward {
	return a.store.ListPortForwards()
}

func (a *App) UpsertPortForward(f config.PortForward) error {
	return a.store.UpsertPortForward(f)
}

func (a *App) DeletePortForward(id string) error {
	_ = a.fwd.Stop(id)
	return a.store.DeletePortForward(id)
}

func (a *App) StartPortForward(id string) error {
	f, err := a.store.GetPortForward(id)
	if err != nil {
		return err
	}
	srv, err := a.store.GetServer(f.ServerID)
	if err != nil {
		return err
	}
	return a.fwd.Start(a.ctx, f, srv)
}

func (a *App) StopPortForward(id string) error {
	return a.fwd.Stop(id)
}

func (a *App) PortForwardStatus(id string) forward.Status {
	return a.fwd.Status(id)
}

// --- Logs ---

func (a *App) RecentLogs() []string {
	if a.ring == nil {
		return nil
	}
	return a.ring.Snapshot()
}

func (a *App) OpenLogDirectory() error {
	logDir := filepath.Join(userLocalAppData(), "ssh_gun", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	if err := shellopen.Open(logDir); err != nil {
		return fmt.Errorf("open log directory: %w", err)
	}
	return nil
}

// --- Config export / import ---

func (a *App) OpenConfigDirectory() error {
	return openConfigDirectory(defaultConfigPath(), shellopen.Open)
}

func (a *App) ExportConfig(passphrase string) (string, error) {
	data, err := a.store.ExportBytes(passphrase)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出配置",
		DefaultFilename: "飞梭-export.toml",
		Filters: []runtime.FileFilter{
			{DisplayName: "飞梭导出", Pattern: "*.toml"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ImportConfig loads an export file. mode is "merge" or "replace".
func (a *App) ImportConfig(passphrase string, mode string) (map[string]any, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入配置",
		Filters: []runtime.FileFilter{
			{DisplayName: "飞梭导出", Pattern: "*.toml"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return map[string]any{"cancelled": true}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	incoming, meta, err := config.ParseExport(data, passphrase)
	if err != nil {
		return nil, err
	}

	a.stopAllRuntime()

	var sum config.ImportSummary
	switch mode {
	case "replace":
		sum, err = a.store.ImportReplace(incoming)
	case "merge":
		sum, err = a.store.ImportMerge(incoming)
	default:
		a.startAutostart()
		return nil, fmt.Errorf("unknown import mode %q", mode)
	}
	if err != nil {
		a.startAutostart()
		return nil, err
	}
	sum.SecretsMode = meta.SecretsMode
	a.startAutostart()

	return map[string]any{
		"serversAdded":      sum.ServersAdded,
		"serversUpdated":    sum.ServersUpdated,
		"mappingsAdded":     sum.MappingsAdded,
		"mappingsUpdated":   sum.MappingsUpdated,
		"forwardsAdded":     sum.ForwardsAdded,
		"forwardsUpdated":   sum.ForwardsUpdated,
		"missingServerRefs": sum.MissingServerRefs,
		"secretsMode":       sum.SecretsMode,
	}, nil
}

func (a *App) GetTheme() string {
	theme, err := config.NormalizeTheme(a.store.UI().Theme)
	if err != nil {
		return "system"
	}
	return theme
}

func (a *App) SetTheme(theme string) error {
	normalized, err := config.NormalizeTheme(theme)
	if err != nil {
		return err
	}
	ui := a.store.UI()
	ui.Theme = normalized
	return a.store.UpdateUI(ui)
}

func (a *App) GetLanguage() string {
	language, err := config.NormalizeLanguage(a.store.UI().Language)
	if err != nil {
		return "zh-CN"
	}
	return language
}

func (a *App) SetLanguage(language string) error {
	normalized, err := config.NormalizeLanguage(language)
	if err != nil {
		return err
	}
	ui := a.store.UI()
	ui.Language = normalized
	if err := a.store.UpdateUI(ui); err != nil {
		return err
	}
	appTray.refreshLabels(normalized)
	return nil
}

func (a *App) GetSyncBackend() string {
	return a.store.SyncBackend()
}

func (a *App) SetSyncBackend(backend string) error {
	return a.store.UpdateSyncBackend(backend)
}

func (a *App) Greet(name string) string {
	return fmt.Sprintf("hello %s", name)
}
