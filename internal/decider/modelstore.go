package decider

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SecretBox seals and opens a model's own API key. The app passes the same
// cipher that encrypts provider secrets; this package imports none of it.
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// modelsFileName is the decision-model document inside <dataDir>/decider/,
// next to the ledger. Like providers.json it is separate from the settings
// document: its own lock, its own atomic write, encrypted secrets.
const modelsFileName = "models.json"

var modelIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ModelStore is the thread-safe, file-backed list of decision models. With no
// directory it keeps them in memory only (tests).
type ModelStore struct {
	path string
	box  SecretBox
	now  func() time.Time

	mu  sync.RWMutex
	cur []ModelInstance
	// existed records whether the file was on disk when the store opened, so
	// the one-time seeding of a first model never runs over a user's list.
	existed bool
}

// modelFile is the on-disk shape: the API-facing struct plus the encrypted
// secrets its json:"-" tag hides.
type modelFile struct {
	ModelInstance
	SecretsEnc map[string]string `json:"secretsEnc,omitempty"`
}

// OpenModelStore loads <dir>/models.json. A missing file is an empty store; a
// corrupt one is moved aside (models.json.corrupt-<unix>) with a warning, so a
// bad edit never blocks boot and is never silently overwritten.
func OpenModelStore(dir string, box SecretBox, logger *slog.Logger) *ModelStore {
	if logger == nil {
		logger = slog.Default()
	}
	s := &ModelStore{box: box, now: time.Now}
	if dir == "" {
		return s
	}
	s.path = filepath.Join(dir, modelsFileName)
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	s.existed = true
	if err != nil {
		logger.Warn("decision models unreadable; starting with none", "path", s.path, "error", err)
		return s
	}
	var files []modelFile
	if err := json.Unmarshal(data, &files); err != nil {
		aside := s.path + ".corrupt-" + strconv.FormatInt(time.Now().Unix(), 10)
		_ = os.Rename(s.path, aside)
		logger.Warn("decision models file is corrupt; moved aside, starting with none", "path", s.path, "movedTo", aside, "error", err)
		return s
	}
	for _, f := range files {
		m := f.ModelInstance
		m.SecretsEnc = f.SecretsEnc
		if m.ID == "" {
			continue
		}
		s.cur = append(s.cur, m)
	}
	return s
}

// Existed reports whether the models file was on disk at open.
func (s *ModelStore) Existed() bool { return s.existed }

// List returns a copy of every model, in stored order.
func (s *ModelStore) List() []ModelInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ModelInstance, len(s.cur))
	for i, m := range s.cur {
		out[i] = cloneModel(m)
	}
	return out
}

// Get returns one model (with its encrypted secrets; process-internal use).
func (s *ModelStore) Get(id string) (ModelInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.cur {
		if m.ID == id {
			return cloneModel(m), true
		}
	}
	return ModelInstance{}, false
}

// Secret returns a model's decrypted secret, "" when unset or undecryptable.
func (s *ModelStore) Secret(id, key string) string {
	m, ok := s.Get(id)
	if !ok || m.SecretsEnc[key] == "" || s.box == nil {
		return ""
	}
	plain, err := s.box.Decrypt(m.SecretsEnc[key])
	if err != nil {
		return ""
	}
	return plain
}

// Upsert creates a model (in.ID empty or unused) or updates one, after
// checking it against its backend. A new model gets a DM<n> id unless in.ID
// names a free, well-formed one.
func (s *ModelStore) Upsert(in ModelInput) (ModelInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	in.ID = strings.TrimSpace(in.ID)
	idx := -1
	for i, m := range s.cur {
		if in.ID != "" && m.ID == in.ID {
			idx = i
			break
		}
	}
	var existing *ModelInstance
	if idx >= 0 {
		e := cloneModel(s.cur[idx])
		existing = &e
	} else if in.ID != "" && !modelIDRe.MatchString(in.ID) {
		return ModelInstance{}, fmt.Errorf("invalid decision model id %q (letters, digits, '.', '_', '-')", in.ID)
	}
	in, _, err := normalizeModelInput(in, existing)
	if err != nil {
		return ModelInstance{}, err
	}

	now := s.now()
	var entry ModelInstance
	if existing != nil {
		entry = *existing
	} else {
		entry = ModelInstance{ID: in.ID, CreatedAt: now, SecretsEnc: map[string]string{}}
		if entry.ID == "" {
			entry.ID = s.nextID()
		}
	}
	entry.Label = in.Label
	entry.Backend = in.Backend
	entry.Enabled = in.Enabled
	entry.Model = in.Model
	entry.Credentials = in.Credentials
	entry.ProviderInstanceID = in.ProviderInstanceID
	entry.BaseURL = in.BaseURL
	entry.TimeoutMs = in.TimeoutMs
	entry.ContextTokens = in.ContextTokens
	entry.Config = in.Config
	entry.UpdatedAt = now
	if entry.SecretsEnc == nil {
		entry.SecretsEnc = map[string]string{}
	}
	for k, v := range in.Secrets {
		if v == "" {
			delete(entry.SecretsEnc, k)
			continue
		}
		if s.box == nil {
			return ModelInstance{}, fmt.Errorf("secret storage is not available")
		}
		enc, err := s.box.Encrypt(v)
		if err != nil {
			return ModelInstance{}, fmt.Errorf("encrypt secret %q: %w", k, err)
		}
		entry.SecretsEnc[k] = enc
	}

	next := make([]ModelInstance, len(s.cur), len(s.cur)+1)
	copy(next, s.cur)
	if idx >= 0 {
		next[idx] = entry
	} else {
		next = append(next, entry)
	}
	if err := s.persist(next); err != nil {
		return ModelInstance{}, err
	}
	s.cur = next
	return cloneModel(entry), nil
}

// Delete removes a model.
func (s *ModelStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]ModelInstance, 0, len(s.cur))
	found := false
	for _, m := range s.cur {
		if m.ID == id {
			found = true
			continue
		}
		next = append(next, m)
	}
	if !found {
		return fmt.Errorf("decision model %q not found", id)
	}
	if err := s.persist(next); err != nil {
		return err
	}
	s.cur = next
	return nil
}

// nextID returns DM<n>, one above the highest existing DM number. Called with
// s.mu held.
func (s *ModelStore) nextID() string {
	highest := 0
	for _, m := range s.cur {
		if n, ok := strings.CutPrefix(m.ID, "DM"); ok {
			if v, err := strconv.Atoi(n); err == nil && v > highest {
				highest = v
			}
		}
	}
	return "DM" + strconv.Itoa(highest+1)
}

// persist writes list atomically. Called with s.mu held.
func (s *ModelStore) persist(list []ModelInstance) error {
	if s.path == "" {
		return nil
	}
	files := make([]modelFile, len(list))
	for i, m := range list {
		files[i] = modelFile{ModelInstance: m, SecretsEnc: m.SecretsEnc}
	}
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.path, data); err != nil {
		return fmt.Errorf("save decision models: %w", err)
	}
	s.existed = true
	return nil
}

func cloneModel(m ModelInstance) ModelInstance {
	m.Config = maps.Clone(m.Config)
	m.SecretsEnc = maps.Clone(m.SecretsEnc)
	return m
}
