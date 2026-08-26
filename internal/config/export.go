package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/argon2"
)

const (
	ExportFormatName    = "ssh_gun-export"
	ExportFormatVersion = 1

	SecretsOmitted   = "omitted"
	SecretsEncrypted = "encrypted"
)

// ExportMeta describes an export package after parsing.
type ExportMeta struct {
	SecretsMode string
	ExportedAt  string
}

type exportDocument struct {
	Format        string        `toml:"format"`
	FormatVersion int           `toml:"format_version"`
	ExportedAt    string        `toml:"exported_at"`
	Secrets       string        `toml:"secrets"`
	Salt          string        `toml:"salt,omitempty"`
	Nonce         string        `toml:"nonce,omitempty"`
	SecretsBlob   string        `toml:"secrets_blob,omitempty"`
	SyncBackend   string        `toml:"sync_backend"`
	Servers       []Server      `toml:"server"`
	SyncMappings  []SyncMapping `toml:"sync_mapping"`
	PortForwards  []PortForward `toml:"port_forward"`
	UI            UIState       `toml:"ui"`
}

type serverSecrets struct {
	Password      string `json:"password,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`
}

// Export builds a portable export TOML from an in-memory (decrypted) File.
// Empty passphrase strips secrets; non-empty encrypts them with Argon2id + AES-GCM.
func Export(file File, passphrase string) ([]byte, error) {
	syncBackend, err := NormalizeSyncBackend(file.SyncBackend)
	if err != nil {
		return nil, err
	}
	doc := exportDocument{
		Format:        ExportFormatName,
		FormatVersion: ExportFormatVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		SyncBackend:   syncBackend,
		Servers:       append([]Server(nil), file.Servers...),
		SyncMappings:  append([]SyncMapping(nil), file.SyncMappings...),
		PortForwards:  append([]PortForward(nil), file.PortForwards...),
		UI:            file.UI,
	}
	for i := range doc.SyncMappings {
		doc.SyncMappings[i].Excludes = append([]string(nil), file.SyncMappings[i].Excludes...)
	}

	if passphrase == "" {
		doc.Secrets = SecretsOmitted
		for i := range doc.Servers {
			doc.Servers[i].Password = ""
			doc.Servers[i].KeyPassphrase = ""
		}
	} else {
		doc.Secrets = SecretsEncrypted
		secrets := make(map[string]serverSecrets, len(doc.Servers))
		for i := range doc.Servers {
			id := doc.Servers[i].ID
			secrets[id] = serverSecrets{
				Password:      doc.Servers[i].Password,
				KeyPassphrase: doc.Servers[i].KeyPassphrase,
			}
			doc.Servers[i].Password = ""
			doc.Servers[i].KeyPassphrase = ""
		}
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}
		nonce := make([]byte, 12)
		if _, err := rand.Read(nonce); err != nil {
			return nil, err
		}
		plain, err := json.Marshal(secrets)
		if err != nil {
			return nil, err
		}
		blob, err := sealSecrets(passphrase, salt, nonce, plain)
		if err != nil {
			return nil, err
		}
		doc.Salt = base64.StdEncoding.EncodeToString(salt)
		doc.Nonce = base64.StdEncoding.EncodeToString(nonce)
		doc.SecretsBlob = base64.StdEncoding.EncodeToString(blob)
	}

	return toml.Marshal(doc)
}

// ParseExport decodes an export package and restores secrets when encrypted.
func ParseExport(data []byte, passphrase string) (File, ExportMeta, error) {
	var doc exportDocument
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return File{}, ExportMeta{}, fmt.Errorf("decode export: %w", err)
	}
	if doc.Format != ExportFormatName {
		return File{}, ExportMeta{}, fmt.Errorf("not an ssh_gun export (format %q)", doc.Format)
	}
	if doc.FormatVersion != ExportFormatVersion {
		return File{}, ExportMeta{}, fmt.Errorf("unsupported export format version %d", doc.FormatVersion)
	}
	meta := ExportMeta{SecretsMode: doc.Secrets, ExportedAt: doc.ExportedAt}

	file := File{
		Version:      CurrentVersion,
		SyncBackend:  doc.SyncBackend,
		Servers:      doc.Servers,
		SyncMappings: doc.SyncMappings,
		PortForwards: doc.PortForwards,
		UI:           doc.UI,
	}

	switch doc.Secrets {
	case SecretsOmitted, "":
		meta.SecretsMode = SecretsOmitted
		for i := range file.Servers {
			file.Servers[i].Password = ""
			file.Servers[i].KeyPassphrase = ""
		}
	case SecretsEncrypted:
		if passphrase == "" {
			return File{}, meta, errors.New("export contains encrypted secrets; passphrase required")
		}
		salt, err := base64.StdEncoding.DecodeString(doc.Salt)
		if err != nil {
			return File{}, meta, fmt.Errorf("invalid salt: %w", err)
		}
		nonce, err := base64.StdEncoding.DecodeString(doc.Nonce)
		if err != nil {
			return File{}, meta, fmt.Errorf("invalid nonce: %w", err)
		}
		blob, err := base64.StdEncoding.DecodeString(doc.SecretsBlob)
		if err != nil {
			return File{}, meta, fmt.Errorf("invalid secrets blob: %w", err)
		}
		plain, err := openSecrets(passphrase, salt, nonce, blob)
		if err != nil {
			return File{}, meta, errors.New("incorrect passphrase or corrupt secrets")
		}
		var secrets map[string]serverSecrets
		if err := json.Unmarshal(plain, &secrets); err != nil {
			return File{}, meta, fmt.Errorf("decode secrets: %w", err)
		}
		for i := range file.Servers {
			if s, ok := secrets[file.Servers[i].ID]; ok {
				file.Servers[i].Password = s.Password
				file.Servers[i].KeyPassphrase = s.KeyPassphrase
			}
		}
	default:
		return File{}, meta, fmt.Errorf("unknown secrets mode %q", doc.Secrets)
	}
	return file, meta, nil
}

func sealSecrets(passphrase string, salt, nonce, plaintext []byte) ([]byte, error) {
	key := argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce size %d, want %d", len(nonce), gcm.NonceSize())
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nil
}

func openSecrets(passphrase string, salt, nonce, ciphertext []byte) ([]byte, error) {
	key := argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
