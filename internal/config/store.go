package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Store is the in-memory authority for configuration with atomic TOML persistence.
type Store struct {
	mu   sync.Mutex
	path string
	file File
}

// Open loads config from path (or recovers from .bak / starts empty).
func Open(path string) (*Store, error) {
	s := &Store{path: path, file: File{Version: CurrentVersion, SyncBackend: SyncBackendSFTP}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	f, err := decodeFile(data)
	if err != nil {
		bak := s.path + ".bak"
		bakData, bakErr := os.ReadFile(bak)
		if bakErr != nil {
			_ = os.Rename(s.path, s.path+".bad")
			s.file = File{Version: CurrentVersion, SyncBackend: SyncBackendSFTP}
			return nil
		}
		f, err = decodeFile(bakData)
		if err != nil {
			_ = os.Rename(s.path, s.path+".bad")
			s.file = File{Version: CurrentVersion, SyncBackend: SyncBackendSFTP}
			return nil
		}
		_ = os.WriteFile(s.path, bakData, 0o600)
	}
	s.file = f
	return nil
}

func decodeFile(data []byte) (File, error) {
	var f File
	if _, err := toml.Decode(string(data), &f); err != nil {
		return File{}, err
	}
	if f.Version == 0 {
		f.Version = CurrentVersion
	}
	syncBackend, err := NormalizeSyncBackend(f.SyncBackend)
	if err != nil {
		return File{}, err
	}
	f.SyncBackend = syncBackend
	if err := decryptSecrets(&f); err != nil {
		return File{}, err
	}
	return f, nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	toWrite := cloneFile(s.file)
	toWrite.Version = CurrentVersion
	if err := encryptSecrets(&toWrite); err != nil {
		return err
	}
	encoded, err := toml.Marshal(toWrite)
	if err != nil {
		return err
	}

	if _, err := os.Stat(s.path); err == nil {
		_ = copyFile(s.path, s.path+".bak")
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func cloneFile(f File) File {
	out := f
	out.Servers = append([]Server(nil), f.Servers...)
	out.SyncMappings = append([]SyncMapping(nil), f.SyncMappings...)
	out.PortForwards = append([]PortForward(nil), f.PortForwards...)
	for i := range out.SyncMappings {
		out.SyncMappings[i].Excludes = append([]string(nil), f.SyncMappings[i].Excludes...)
	}
	return out
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func encryptSecrets(f *File) error {
	for i := range f.Servers {
		enc, err := protect(f.Servers[i].Password)
		if err != nil {
			return err
		}
		f.Servers[i].Password = enc
		enc, err = protect(f.Servers[i].KeyPassphrase)
		if err != nil {
			return err
		}
		f.Servers[i].KeyPassphrase = enc
	}
	return nil
}

func decryptSecrets(f *File) error {
	for i := range f.Servers {
		plain, err := unprotect(f.Servers[i].Password)
		if err != nil {
			return err
		}
		f.Servers[i].Password = plain
		plain, err = unprotect(f.Servers[i].KeyPassphrase)
		if err != nil {
			return err
		}
		f.Servers[i].KeyPassphrase = plain
	}
	return nil
}

func (s *Store) UpsertServer(srv Server) error {
	normalized, err := NormalizeServer(srv)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	replaced := false
	for i := range s.file.Servers {
		if s.file.Servers[i].ID == normalized.ID {
			// Preserve secrets if UI sent mask / empty meaning "unchanged"
			if normalized.Password == "" || normalized.Password == SecretMask {
				normalized.Password = s.file.Servers[i].Password
			}
			if normalized.KeyPassphrase == "" || normalized.KeyPassphrase == SecretMask {
				normalized.KeyPassphrase = s.file.Servers[i].KeyPassphrase
			}
			s.file.Servers[i] = normalized
			replaced = true
			break
		}
	}
	if !replaced {
		s.file.Servers = append(s.file.Servers, normalized)
	}
	return s.saveLocked()
}

func (s *Store) GetServer(id string) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, srv := range s.file.Servers {
		if srv.ID == id {
			return srv, nil
		}
	}
	return Server{}, fmt.Errorf("server %q not found", id)
}

func (s *Store) ListServersForUI() []ServerUI {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ServerUI, 0, len(s.file.Servers))
	for _, srv := range s.file.Servers {
		ui := ServerUI{
			ID:            srv.ID,
			Name:          srv.Name,
			Host:          srv.Host,
			Port:          srv.Port,
			User:          srv.User,
			AuthType:      srv.AuthType,
			KeyPath:       srv.KeyPath,
			HostKeyPolicy: srv.HostKeyPolicy,
			HasPassword:   srv.Password != "",
			HasPassphrase: srv.KeyPassphrase != "",
		}
		if srv.Password != "" {
			ui.Password = SecretMask
		}
		if srv.KeyPassphrase != "" {
			ui.KeyPassphrase = SecretMask
		}
		out = append(out, ui)
	}
	return out
}

func (s *Store) DeleteServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var maps, fwds []string
	for _, m := range s.file.SyncMappings {
		if m.ServerID == id {
			maps = append(maps, m.Name)
		}
	}
	for _, f := range s.file.PortForwards {
		if f.ServerID == id {
			fwds = append(fwds, f.Name)
		}
	}
	if len(maps) > 0 || len(fwds) > 0 {
		return &InUseError{Mappings: maps, Forwards: fwds}
	}
	filtered := s.file.Servers[:0]
	for _, srv := range s.file.Servers {
		if srv.ID != id {
			filtered = append(filtered, srv)
		}
	}
	s.file.Servers = filtered
	return s.saveLocked()
}

func (s *Store) UpsertSyncMapping(m SyncMapping) error {
	normalized, err := NormalizeSyncMapping(m)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	replaced := false
	for i := range s.file.SyncMappings {
		if s.file.SyncMappings[i].ID == normalized.ID {
			s.file.SyncMappings[i] = normalized
			replaced = true
			break
		}
	}
	if !replaced {
		s.file.SyncMappings = append(s.file.SyncMappings, normalized)
	}
	return s.saveLocked()
}

func (s *Store) GetSyncMapping(id string) (SyncMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.file.SyncMappings {
		if m.ID == id {
			return m, nil
		}
	}
	return SyncMapping{}, fmt.Errorf("sync mapping %q not found", id)
}

func (s *Store) ListSyncMappings() []SyncMapping {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SyncMapping, len(s.file.SyncMappings))
	copy(out, s.file.SyncMappings)
	return out
}

func (s *Store) DeleteSyncMapping(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.file.SyncMappings[:0]
	for _, m := range s.file.SyncMappings {
		if m.ID != id {
			filtered = append(filtered, m)
		}
	}
	s.file.SyncMappings = filtered
	return s.saveLocked()
}

func (s *Store) UpsertPortForward(f PortForward) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalized, err := NormalizePortForward(f)
	if err != nil {
		return err
	}
	f = normalized
	replaced := false
	for i := range s.file.PortForwards {
		if s.file.PortForwards[i].ID == f.ID {
			s.file.PortForwards[i] = f
			replaced = true
			break
		}
	}
	if !replaced {
		s.file.PortForwards = append(s.file.PortForwards, f)
	}
	return s.saveLocked()
}

// NormalizePortForward applies defaults and validates a port forward.
func NormalizePortForward(f PortForward) (PortForward, error) {
	f.Name = strings.TrimSpace(f.Name)
	f.ServerID = strings.TrimSpace(f.ServerID)
	f.LocalAddr = strings.TrimSpace(f.LocalAddr)
	f.RemoteHost = strings.TrimSpace(f.RemoteHost)
	if f.Name == "" {
		return PortForward{}, fmt.Errorf("port forward name is required")
	}
	if f.ServerID == "" {
		return PortForward{}, fmt.Errorf("port forward server is required")
	}
	switch f.Type {
	case "":
		f.Type = ForwardTypeLocal
	case ForwardTypeLocal, ForwardTypeRemote, ForwardTypeDynamic:
		// ok
	default:
		return PortForward{}, fmt.Errorf("invalid port forward type %q", f.Type)
	}
	if f.LocalPort < 1 || f.LocalPort > 65535 {
		return PortForward{}, fmt.Errorf("local port must be between 1 and 65535")
	}
	switch f.Type {
	case ForwardTypeDynamic:
		f.RemotePort = 0
		if f.LocalAddr == "" {
			f.LocalAddr = "127.0.0.1"
		}
	case ForwardTypeRemote:
		if f.LocalAddr == "" {
			return PortForward{}, fmt.Errorf("local address is required")
		}
		if f.RemotePort < 1 || f.RemotePort > 65535 {
			return PortForward{}, fmt.Errorf("remote port must be between 1 and 65535")
		}
		if f.RemoteHost == "" {
			f.RemoteHost = "127.0.0.1"
		}
	default: // local
		if f.RemoteHost == "" {
			return PortForward{}, fmt.Errorf("remote host is required")
		}
		if f.RemotePort < 1 || f.RemotePort > 65535 {
			return PortForward{}, fmt.Errorf("remote port must be between 1 and 65535")
		}
		if f.LocalAddr == "" {
			f.LocalAddr = "127.0.0.1"
		}
	}
	return f, nil
}

func (s *Store) GetPortForward(id string) (PortForward, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.file.PortForwards {
		if f.ID == id {
			return f, nil
		}
	}
	return PortForward{}, fmt.Errorf("port forward %q not found", id)
}

func (s *Store) ListPortForwards() []PortForward {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PortForward, len(s.file.PortForwards))
	copy(out, s.file.PortForwards)
	return out
}

func (s *Store) DeletePortForward(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.file.PortForwards[:0]
	for _, f := range s.file.PortForwards {
		if f.ID != id {
			filtered = append(filtered, f)
		}
	}
	s.file.PortForwards = filtered
	return s.saveLocked()
}

func (s *Store) UpdateUI(ui UIState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file.UI = ui
	return s.saveLocked()
}

func (s *Store) UI() UIState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.UI
}

func (s *Store) SyncBackend() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	backend, err := NormalizeSyncBackend(s.file.SyncBackend)
	if err != nil {
		return SyncBackendSFTP
	}
	return backend
}

func (s *Store) UpdateSyncBackend(backend string) error {
	normalized, err := NormalizeSyncBackend(backend)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file.SyncBackend = normalized
	return s.saveLocked()
}

// ImportSummary reports what changed during an import.
type ImportSummary struct {
	ServersAdded      int      `json:"serversAdded"`
	ServersUpdated    int      `json:"serversUpdated"`
	MappingsAdded     int      `json:"mappingsAdded"`
	MappingsUpdated   int      `json:"mappingsUpdated"`
	ForwardsAdded     int      `json:"forwardsAdded"`
	ForwardsUpdated   int      `json:"forwardsUpdated"`
	MissingServerRefs []string `json:"missingServerRefs"`
	SecretsMode       string   `json:"secretsMode"`
}

// ExportBytes serializes the current in-memory config as a portable export.
func (s *Store) ExportBytes(passphrase string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Export(cloneFile(s.file), passphrase)
}

// ImportMerge upserts servers/mappings/forwards by ID; keeps local UI.
func (s *Store) ImportMerge(incoming File) (ImportSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum := ImportSummary{}
	s.file.Servers, sum.ServersAdded, sum.ServersUpdated = mergeServers(s.file.Servers, incoming.Servers)
	s.file.SyncMappings, sum.MappingsAdded, sum.MappingsUpdated = mergeMappings(s.file.SyncMappings, incoming.SyncMappings)
	s.file.PortForwards, sum.ForwardsAdded, sum.ForwardsUpdated = mergeForwards(s.file.PortForwards, incoming.PortForwards)
	sum.MissingServerRefs = missingServerRefs(s.file)
	if err := s.saveLocked(); err != nil {
		return ImportSummary{}, err
	}
	return sum, nil
}

// ImportReplace replaces servers/mappings/forwards; applies incoming UI when non-zero.
func (s *Store) ImportReplace(incoming File) (ImportSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	syncBackend, err := NormalizeSyncBackend(incoming.SyncBackend)
	if err != nil {
		return ImportSummary{}, err
	}
	sum := ImportSummary{
		ServersAdded:  len(incoming.Servers),
		MappingsAdded: len(incoming.SyncMappings),
		ForwardsAdded: len(incoming.PortForwards),
	}
	keepUI := s.file.UI
	s.file.SyncBackend = syncBackend
	s.file.Servers = append([]Server(nil), incoming.Servers...)
	s.file.SyncMappings = append([]SyncMapping(nil), incoming.SyncMappings...)
	for i := range s.file.SyncMappings {
		s.file.SyncMappings[i].Excludes = append([]string(nil), incoming.SyncMappings[i].Excludes...)
	}
	s.file.PortForwards = append([]PortForward(nil), incoming.PortForwards...)
	if incoming.UI != (UIState{}) {
		s.file.UI = incoming.UI
	} else {
		s.file.UI = keepUI
	}
	sum.MissingServerRefs = missingServerRefs(s.file)
	if err := s.saveLocked(); err != nil {
		return ImportSummary{}, err
	}
	return sum, nil
}

func mergeServers(existing, incoming []Server) (out []Server, added, updated int) {
	out = append([]Server(nil), existing...)
	index := map[string]int{}
	for i, s := range out {
		index[s.ID] = i
	}
	for _, srv := range incoming {
		if i, ok := index[srv.ID]; ok {
			out[i] = srv
			updated++
		} else {
			out = append(out, srv)
			index[srv.ID] = len(out) - 1
			added++
		}
	}
	return out, added, updated
}

func mergeMappings(existing, incoming []SyncMapping) (out []SyncMapping, added, updated int) {
	out = append([]SyncMapping(nil), existing...)
	for i := range out {
		out[i].Excludes = append([]string(nil), existing[i].Excludes...)
	}
	index := map[string]int{}
	for i, m := range out {
		index[m.ID] = i
	}
	for _, m := range incoming {
		cp := m
		cp.Excludes = append([]string(nil), m.Excludes...)
		if i, ok := index[m.ID]; ok {
			out[i] = cp
			updated++
		} else {
			out = append(out, cp)
			index[m.ID] = len(out) - 1
			added++
		}
	}
	return out, added, updated
}

func mergeForwards(existing, incoming []PortForward) (out []PortForward, added, updated int) {
	out = append([]PortForward(nil), existing...)
	index := map[string]int{}
	for i, f := range out {
		index[f.ID] = i
	}
	for _, f := range incoming {
		if i, ok := index[f.ID]; ok {
			out[i] = f
			updated++
		} else {
			out = append(out, f)
			index[f.ID] = len(out) - 1
			added++
		}
	}
	return out, added, updated
}

func missingServerRefs(f File) []string {
	ids := make(map[string]struct{}, len(f.Servers))
	for _, s := range f.Servers {
		ids[s.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	var missing []string
	add := func(serverID, kind, name string) {
		if serverID == "" {
			return
		}
		if _, ok := ids[serverID]; ok {
			return
		}
		key := kind + ":" + name + "->" + serverID
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		missing = append(missing, fmt.Sprintf("%s %q references missing server %q", kind, name, serverID))
	}
	for _, m := range f.SyncMappings {
		add(m.ServerID, "mapping", m.Name)
	}
	for _, fwd := range f.PortForwards {
		add(fwd.ServerID, "forward", fwd.Name)
	}
	return missing
}
