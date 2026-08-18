package settings

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// providersFileName is the provider-instance document inside the data
// directory. Deliberately separate from settings.json (K2, _Docs/71 §2.4):
// its own lock, its own atomic write, no growth pressure on Patch.
const providersFileName = "providers.json"

// instanceIDRe constrains a provider instance id the same way providerIDRe
// constrains a custom-provider id: a clean identifier, since the id doubles
// as both a lookup key and (for migrated defaults) a provider kind id.
var instanceIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ProviderStore is the thread-safe, file-backed holder of provider instances.
// It is a second, independent store from Store (settings.json) — see the
// package doc in store.go and _Docs/71 §2.4 for why the two are not merged.
type ProviderStore struct {
	path   string
	cipher Cipher

	mu  sync.RWMutex
	cur []ProviderInstance
	// fileExisted records whether providers.json was already on disk when
	// Open ran, as opposed to being freshly created by it. EnsureMigrated
	// uses this (not len(cur)/nil-ness, which can't distinguish "freshly
	// created" from "loaded but genuinely empty") to decide whether it is
	// allowed to write: migration must run at most once, exactly when the
	// file did not previously exist.
	fileExisted bool
}

// OpenProviderStore loads providers.json from dataDir, creating an empty
// document if absent. It does not run migration — call EnsureMigrated
// separately once the caller has a Settings value + decrypt function to
// migrate from.
func OpenProviderStore(dataDir string, cipher Cipher) (*ProviderStore, error) {
	s := &ProviderStore{
		path:   filepath.Join(dataDir, providersFileName),
		cipher: cipher,
	}

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		if err := s.persist(nil); err != nil {
			return nil, err
		}
		s.cur = []ProviderInstance{}
		s.fileExisted = false
		return s, nil
	}
	if err != nil {
		return nil, err
	}

	var files []providerInstanceFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("parse %s: %w", providersFileName, err)
	}
	instances := make([]ProviderInstance, len(files))
	for i, f := range files {
		instances[i] = f.toInstance()
	}

	deduped, dropped := dedupeInstancesLastWins(instances)
	if len(dropped) > 0 {
		// providers.json had duplicate ids on disk (a known migration bug,
		// see MigrateFromSettings). The last entry per id wins on repair
		// because that already matches live runtime behavior: providers.Registry
		// builds its lookup map from this same list in order, so the last
		// duplicate is the one every agent has actually been resolving
		// against. Repairing to anything else would silently switch the
		// provider agents use out from under them.
		for _, id := range dropped {
			slog.Warn("providers.json: dropped duplicate provider instance id on load, keeping last occurrence", "id", id)
		}
		if err := writeProviderInstanceFiles(s.path, deduped); err != nil {
			return nil, fmt.Errorf("repair duplicate provider instance ids in %s: %w", providersFileName, err)
		}
		instances = deduped
	}

	s.cur = instances
	s.fileExisted = true
	return s, nil
}

// dedupeInstancesLastWins collapses duplicate-id entries in list, keeping the
// LAST occurrence of each id (matching providers.Registry's map-assignment
// order, so a repair never changes which instance is actually in effect at
// runtime) and preserving the first-seen position for the surviving entry.
// Returns the deduped list and the ids that had duplicates dropped.
func dedupeInstancesLastWins(list []ProviderInstance) ([]ProviderInstance, []string) {
	lastByID := make(map[string]ProviderInstance, len(list))
	firstPos := make(map[string]int, len(list))
	order := make([]string, 0, len(list))
	seenDup := make(map[string]bool)
	for i, p := range list {
		if _, ok := firstPos[p.ID]; !ok {
			firstPos[p.ID] = i
			order = append(order, p.ID)
		} else {
			seenDup[p.ID] = true
		}
		lastByID[p.ID] = p
	}
	if len(seenDup) == 0 {
		return list, nil
	}
	out := make([]ProviderInstance, 0, len(order))
	for _, id := range order {
		out = append(out, lastByID[id])
	}
	dropped := make([]string, 0, len(seenDup))
	for id := range seenDup {
		dropped = append(dropped, id)
	}
	sort.Strings(dropped)
	return out, dropped
}

// Path returns the absolute path of providers.json on disk.
func (s *ProviderStore) Path() string { return s.path }

// List returns a defensive copy of every provider instance, in stored order.
func (s *ProviderStore) List() []ProviderInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProviderInstance, len(s.cur))
	for i, p := range s.cur {
		out[i] = p
		out[i].Config = copyStringMap(p.Config)
		out[i].SecretsEnc = copyStringMap(p.SecretsEnc)
	}
	return out
}

// DTOs returns the masked, client-facing view of every provider instance.
func (s *ProviderStore) DTOs() []ProviderInstanceDTO {
	list := s.List()
	out := make([]ProviderInstanceDTO, len(list))
	for i, p := range list {
		out[i] = p.ToDTO()
	}
	return out
}

// Get returns one provider instance by id (including its encrypted secrets —
// process-internal use only), and whether it was found.
func (s *ProviderStore) Get(id string) (ProviderInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.cur {
		if p.ID == id {
			out := p
			out.Config = copyStringMap(p.Config)
			out.SecretsEnc = copyStringMap(p.SecretsEnc)
			return out, true
		}
	}
	return ProviderInstance{}, false
}

// Secret returns the decrypted plaintext for a given instance/key pair, or ""
// if the instance, the key, or the decrypted value is empty/invalid. Callers
// that need to distinguish "not configured" from "fails to decrypt" should
// inspect the DTO's SecretsSet map instead — this method collapses both to ""
// because every caller site (provider client construction) treats them the
// same way (no usable credential).
func (s *ProviderStore) Secret(id, key string) string {
	s.mu.RLock()
	var enc string
	for _, p := range s.cur {
		if p.ID == id {
			enc = p.SecretsEnc[key]
			break
		}
	}
	s.mu.RUnlock()
	if enc == "" {
		return ""
	}
	plain, err := s.cipher.Decrypt(enc)
	if err != nil {
		return ""
	}
	return plain
}

// Upsert creates a new provider instance (when in.ID is empty, a PRV<n> id is
// generated) or updates an existing one (when in.ID matches a stored
// instance). Secrets follow the write-only convention documented on
// ProviderInstanceInput: absent key = keep, empty value = clear, non-empty
// value = encrypt and replace.
func (s *ProviderStore) Upsert(in ProviderInstanceInput) (ProviderInstance, error) {
	kindID := strings.TrimSpace(in.KindID)
	if kindID == "" {
		return ProviderInstance{}, fmt.Errorf("kindId is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(in.ID)
	idx := -1
	if id != "" {
		if !instanceIDRe.MatchString(id) {
			return ProviderInstance{}, fmt.Errorf("invalid provider instance id (letters, digits, _, -, . — start with a letter or digit)")
		}
		for i, p := range s.cur {
			if p.ID == id {
				idx = i
				break
			}
		}
	} else {
		id = s.nextID()
	}

	var entry ProviderInstance
	if idx >= 0 {
		existing := s.cur[idx]
		if existing.KindID != kindID {
			return ProviderInstance{}, fmt.Errorf("provider instance %q kind cannot be changed (has %q, got %q)", id, existing.KindID, kindID)
		}
		entry = existing
	} else {
		entry = ProviderInstance{
			ID:         id,
			KindID:     kindID,
			SecretsEnc: map[string]string{},
			CreatedAt:  currentTime(),
		}
	}

	entry.Label = strings.TrimSpace(in.Label)
	if entry.Label == "" {
		entry.Label = id
	}
	entry.Icon = in.Icon
	entry.Enabled = in.Enabled
	entry.DefaultModel = strings.TrimSpace(in.DefaultModel)
	entry.Models = in.Models
	entry.Config = copyStringMap(in.Config)
	if entry.SecretsEnc == nil {
		entry.SecretsEnc = map[string]string{}
	}

	for key, val := range in.Secrets {
		if val == "" {
			delete(entry.SecretsEnc, key)
			continue
		}
		enc, err := s.cipher.Encrypt(val)
		if err != nil {
			return ProviderInstance{}, fmt.Errorf("encrypt secret %q: %w", key, err)
		}
		entry.SecretsEnc[key] = enc
	}

	next := make([]ProviderInstance, len(s.cur))
	copy(next, s.cur)
	if idx >= 0 {
		next[idx] = entry
	} else {
		next = append(next, entry)
	}

	if err := s.persist(next); err != nil {
		return ProviderInstance{}, err
	}
	s.cur = next

	out := entry
	out.Config = copyStringMap(entry.Config)
	out.SecretsEnc = copyStringMap(entry.SecretsEnc)
	return out, nil
}

// Delete removes a provider instance by id.
func (s *ProviderStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]ProviderInstance, 0, len(s.cur))
	found := false
	for _, p := range s.cur {
		if p.ID == id {
			found = true
			continue
		}
		next = append(next, p)
	}
	if !found {
		return fmt.Errorf("provider instance %q not found", id)
	}
	if err := s.persist(next); err != nil {
		return err
	}
	s.cur = next
	return nil
}

// nextID generates the next PRV<n> id: n is one greater than the highest
// existing PRV-prefixed numeric suffix, starting at 1. Must be called with
// s.mu already held.
func (s *ProviderStore) nextID() string {
	max := 0
	for _, p := range s.cur {
		n, ok := strings.CutPrefix(p.ID, "PRV")
		if !ok {
			continue
		}
		if v, err := strconv.Atoi(n); err == nil && v > max {
			max = v
		}
	}
	return "PRV" + strconv.Itoa(max+1)
}

// persist writes the provider-instance list to disk atomically (temp file +
// rename), mirroring Store.persist in store.go. A nil/empty list still writes
// a valid empty JSON array so Open never has to special-case "file exists but
// has no instances yet".
//
// It refuses to write a list containing a duplicate id: every write path
// (Upsert, Delete, EnsureMigrated) builds `list` from s.cur plus at most one
// changed/added entry, so a duplicate reaching here means a caller bug, not
// a recoverable runtime condition — it must fail loudly rather than silently
// writing the same ambiguity BUG-1 found on disk.
func (s *ProviderStore) persist(list []ProviderInstance) error {
	seen := make(map[string]bool, len(list))
	for _, p := range list {
		if seen[p.ID] {
			return fmt.Errorf("providers.json: refusing to persist duplicate provider instance id %q", p.ID)
		}
		seen[p.ID] = true
	}
	return writeProviderInstanceFiles(s.path, list)
}

// writeProviderInstanceFiles is the atomic-write primitive shared by persist
// (normal writes, duplicate-checked) and OpenProviderStore's on-load repair
// (writing back an already-deduped list).
func writeProviderInstanceFiles(path string, list []ProviderInstance) error {
	files := make([]providerInstanceFile, len(list))
	for i, p := range list {
		files[i] = p.toFile()
	}
	if files == nil {
		files = []providerInstanceFile{}
	}
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// currentTime is a thin indirection over time.Now so tests can't be broken by
// switching the underlying clock later; kept trivial on purpose.
func currentTime() time.Time { return time.Now() }

// sortInstancesByID is used by tests that need deterministic ordering; kept
// here rather than in the _test.go file so it stays next to the type it sorts.
func sortInstancesByID(list []ProviderInstance) {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
}
