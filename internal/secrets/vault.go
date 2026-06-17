// Package secrets implements a per-workspace, file-backed secret vault. Each
// workspace owns an isolated secrets.json holding API keys, tokens and
// passwords; every value is AES-GCM encrypted at rest and only decrypted on
// demand (for the owner's reveal action or an agent's secret_get tool call).
// Plaintext values are never serialized to disk and never returned by the list
// view.
package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// fileName is the vault document inside the workspace store directory.
const fileName = "secrets.json"

// maxNameLen / maxValueLen bound a single entry so a malformed client cannot
// write an unbounded document.
const (
	maxNameLen  = 128
	maxValueLen = 1 << 16 // 64 KiB
)

// nameRe constrains secret names to a clean identifier so agents can reference
// them unambiguously by key. Must start with a letter; letters/digits/_/-/. after.
var nameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

// ErrNotFound is returned when a named secret does not exist.
var ErrNotFound = errors.New("secret not found")

// ErrInvalidName is returned when a secret name is empty, too long, or malformed.
var ErrInvalidName = errors.New("invalid secret name (use letters, digits, _, -, . starting with a letter)")

// ErrInvalidValue is returned when a secret value is empty or too large.
var ErrInvalidValue = errors.New("invalid secret value (must be non-empty and under 64 KiB)")

// Cipher seals/opens secret values. config.Secret satisfies it; the interface
// keeps this package free of a config import.
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// Entry is one persisted secret. ValueEnc is the AES-GCM ciphertext; the
// plaintext never touches disk.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ValueEnc    string `json:"valueEnc"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Meta is the masked, client-facing view of a secret — never carries the value.
type Meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Vault is the thread-safe, file-backed secret store for one workspace.
type Vault struct {
	path   string
	cipher Cipher

	mu    sync.RWMutex
	items map[string]Entry
}

// Open loads (or creates) the vault under storeDir. A missing file starts empty.
func Open(storeDir string, cipher Cipher) (*Vault, error) {
	v := &Vault{
		path:   filepath.Join(storeDir, fileName),
		cipher: cipher,
		items:  map[string]Entry{},
	}
	data, err := os.ReadFile(v.path)
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	for _, e := range entries {
		v.items[e.Name] = e
	}
	return v, nil
}

// List returns every secret as masked metadata, sorted by name.
func (v *Vault) List() []Meta {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]Meta, 0, len(v.items))
	for _, e := range v.items {
		out = append(out, Meta{
			Name:        e.Name,
			Description: e.Description,
			CreatedAt:   e.CreatedAt,
			UpdatedAt:   e.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns the sorted secret names (no values) — used by the secret_list tool.
func (v *Vault) Names() []Meta { return v.List() }

// Get returns the decrypted value for a name, or ("", false) if absent/undecryptable.
func (v *Vault) Get(name string) (string, bool) {
	v.mu.RLock()
	e, ok := v.items[name]
	v.mu.RUnlock()
	if !ok {
		return "", false
	}
	plain, err := v.cipher.Decrypt(e.ValueEnc)
	if err != nil {
		return "", false
	}
	return plain, true
}

// Set creates or replaces a secret. The name is validated; the value is
// encrypted before it is persisted. Returns the masked metadata of the result.
func (v *Vault) Set(name, value, description string) (Meta, error) {
	if !ValidName(name) {
		return Meta{}, ErrInvalidName
	}
	if value == "" || len(value) > maxValueLen {
		return Meta{}, ErrInvalidValue
	}
	enc, err := v.cipher.Encrypt(value)
	if err != nil {
		return Meta{}, err
	}

	now := time.Now().Unix()
	v.mu.Lock()
	existing, ok := v.items[name]
	created := now
	if ok {
		created = existing.CreatedAt
	}
	entry := Entry{
		Name:        name,
		Description: description,
		ValueEnc:    enc,
		CreatedAt:   created,
		UpdatedAt:   now,
	}
	v.items[name] = entry
	err = v.persistLocked()
	v.mu.Unlock()
	if err != nil {
		return Meta{}, err
	}
	return Meta{
		Name:        entry.Name,
		Description: entry.Description,
		CreatedAt:   entry.CreatedAt,
		UpdatedAt:   entry.UpdatedAt,
	}, nil
}

// Delete removes a secret by name. Returns ErrNotFound when absent.
func (v *Vault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.items[name]; !ok {
		return ErrNotFound
	}
	delete(v.items, name)
	return v.persistLocked()
}

// persistLocked writes the vault to disk atomically (temp file + rename). The
// caller must hold v.mu.
func (v *Vault) persistLocked() error {
	entries := make([]Entry, 0, len(v.items))
	for _, e := range v.items {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

// ValidName reports whether name is a well-formed secret key.
func ValidName(name string) bool {
	return name != "" && len(name) <= maxNameLen && nameRe.MatchString(name)
}
