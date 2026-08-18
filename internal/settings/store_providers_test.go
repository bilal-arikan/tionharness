package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCipher is a trivial reversible Cipher for tests: no real crypto, just
// enough round-tripping to exercise the encrypt/decrypt call paths.
type testCipher struct{}

func (testCipher) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (testCipher) Decrypt(encoded string) (string, error) {
	if len(encoded) < 4 || encoded[:4] != "enc:" {
		return "", errDecryptTest
	}
	return encoded[4:], nil
}

var errDecryptTest = &testDecryptError{}

type testDecryptError struct{}

func (*testDecryptError) Error() string { return "not encrypted by testCipher" }

func openTestStore(t *testing.T) *ProviderStore {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("OpenProviderStore: %v", err)
	}
	return s
}

func TestProviderStore_UpsertGeneratesPRVID(t *testing.T) {
	s := openTestStore(t)

	first, err := s.Upsert(ProviderInstanceInput{KindID: "anthropic", Label: "A"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if first.ID != "PRV1" {
		t.Fatalf("expected PRV1, got %q", first.ID)
	}

	second, err := s.Upsert(ProviderInstanceInput{KindID: "anthropic", Label: "B"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if second.ID != "PRV2" {
		t.Fatalf("expected PRV2, got %q", second.ID)
	}

	// Explicit kind-id-shaped ids (as migration produces) must never collide
	// with or be reused by the PRV<n> generator.
	if _, err := s.Upsert(ProviderInstanceInput{ID: "anthropic", KindID: "anthropic", Label: "C"}); err != nil {
		t.Fatalf("Upsert with explicit id: %v", err)
	}
	third, err := s.Upsert(ProviderInstanceInput{KindID: "anthropic", Label: "D"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if third.ID != "PRV3" {
		t.Fatalf("expected PRV3 (not reusing kind ids), got %q", third.ID)
	}
}

func TestProviderStore_UpsertSecretsWriteOnly(t *testing.T) {
	s := openTestStore(t)

	inst, err := s.Upsert(ProviderInstanceInput{
		ID:     "anthropic",
		KindID: "anthropic",
		Label:  "Anthropic",
		Secrets: map[string]string{
			"key": "sk-live-123",
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := s.Secret(inst.ID, "key"); got != "sk-live-123" {
		t.Fatalf("Secret roundtrip: got %q", got)
	}

	// Update WITHOUT mentioning "key" at all → must be preserved.
	if _, err := s.Upsert(ProviderInstanceInput{ID: "anthropic", KindID: "anthropic", Label: "Anthropic v2"}); err != nil {
		t.Fatalf("Upsert (no secrets field): %v", err)
	}
	if got := s.Secret("anthropic", "key"); got != "sk-live-123" {
		t.Fatalf("expected key preserved when omitted, got %q", got)
	}

	// Update WITH "key" present but empty → must clear.
	if _, err := s.Upsert(ProviderInstanceInput{
		ID: "anthropic", KindID: "anthropic", Label: "Anthropic v3",
		Secrets: map[string]string{"key": ""},
	}); err != nil {
		t.Fatalf("Upsert (clear key): %v", err)
	}
	if got := s.Secret("anthropic", "key"); got != "" {
		t.Fatalf("expected key cleared, got %q", got)
	}
	dto, ok := s.Get("anthropic")
	if !ok {
		t.Fatal("expected instance to still exist after clearing key")
	}
	if dto.ToDTO().SecretsSet["key"] {
		t.Fatal("expected secretsSet[key] false after clearing")
	}
}

func TestProviderStore_UpsertRejectsKindChange(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Upsert(ProviderInstanceInput{ID: "anthropic", KindID: "anthropic"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := s.Upsert(ProviderInstanceInput{ID: "anthropic", KindID: "openai-compat"}); err == nil {
		t.Fatal("expected error changing kindId on an existing instance")
	}
}

func TestProviderStore_UpsertRejectsEmptyKind(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Upsert(ProviderInstanceInput{ID: "x"}); err == nil {
		t.Fatal("expected error for empty kindId")
	}
}

func TestProviderStore_DTONeverLeaksSecret(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Upsert(ProviderInstanceInput{
		ID: "anthropic", KindID: "anthropic",
		Secrets: map[string]string{"key": "sk-super-secret"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dtos := s.DTOs()
	if len(dtos) != 1 {
		t.Fatalf("expected 1 DTO, got %d", len(dtos))
	}
	data, err := json.Marshal(dtos[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "sk-super-secret") {
		t.Fatalf("DTO JSON leaks plaintext secret: %s", data)
	}
	if strings.Contains(string(data), "enc:") {
		t.Fatalf("DTO JSON leaks encrypted secret: %s", data)
	}
	if !strings.Contains(string(data), "secretsSet") {
		t.Fatalf("DTO JSON missing secretsSet: %s", data)
	}

	// Also verify marshaling the raw ProviderInstance (not just the DTO)
	// cannot leak the secret either — SecretsEnc must carry json:"-".
	inst, _ := s.Get("anthropic")
	raw, err := json.Marshal(inst)
	if err != nil {
		t.Fatalf("Marshal ProviderInstance: %v", err)
	}
	if strings.Contains(string(raw), "enc:") || strings.Contains(string(raw), "sk-super-secret") {
		t.Fatalf("ProviderInstance JSON leaks secret: %s", raw)
	}
}

func TestProviderStore_Delete(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Upsert(ProviderInstanceInput{ID: "anthropic", KindID: "anthropic"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Delete("anthropic"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("anthropic"); ok {
		t.Fatal("expected instance gone after Delete")
	}
	if err := s.Delete("anthropic"); err == nil {
		t.Fatal("expected error deleting an already-deleted instance")
	}
}

func TestOpenProviderStore_RepairsDuplicateIDsOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, providersFileName)

	files := []providerInstanceFile{
		{ID: "openrouter", KindID: "openrouter", Label: "OpenRouter", Config: map[string]string{"baseUrl": ""}, SecretsEnc: map[string]string{}, CreatedAt: time.Now()},
		{ID: "openrouter", KindID: "openai-compat", Label: "My OpenRouter", Config: map[string]string{"baseUrl": "https://openrouter.ai/api/v1"}, SecretsEnc: map[string]string{"key": "enc:custom-key"}, CreatedAt: time.Now()},
	}
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("OpenProviderStore: %v", err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance after repair, got %d: %+v", len(list), list)
	}
	inst, ok := s.Get("openrouter")
	if !ok {
		t.Fatal("expected openrouter instance to survive repair")
	}
	// The repair must keep the LAST occurrence — matching providers.Registry's
	// map-assignment order, i.e. the instance actually in effect at runtime
	// before the repair ran.
	if inst.KindID != "openai-compat" {
		t.Fatalf("expected last-occurrence (custom) instance to win, got kindId %q", inst.KindID)
	}
	if inst.Config["baseUrl"] != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected last-occurrence baseUrl to win, got %+v", inst.Config)
	}

	// The repair must be written back to disk...
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after repair: %v", err)
	}
	var onDisk []providerInstanceFile
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("Unmarshal repaired file: %v", err)
	}
	if len(onDisk) != 1 {
		t.Fatalf("expected repaired file to have 1 entry, got %d", len(onDisk))
	}

	// ...so a second open sees no further change.
	reopened, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if list := reopened.List(); len(list) != 1 {
		t.Fatalf("expected 1 instance on reopen, got %d: %+v", len(list), list)
	}
}

func TestProviderStore_PersistRejectsDuplicateID(t *testing.T) {
	s := openTestStore(t)
	dup := []ProviderInstance{
		{ID: "anthropic", KindID: "anthropic", SecretsEnc: map[string]string{}, CreatedAt: time.Now()},
		{ID: "anthropic", KindID: "anthropic-compat", SecretsEnc: map[string]string{}, CreatedAt: time.Now()},
	}
	if err := s.persist(dup); err == nil {
		t.Fatal("expected persist to reject a list containing a duplicate id")
	}
}

func TestProviderStore_AtomicWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("OpenProviderStore: %v", err)
	}
	if _, err := s.Upsert(ProviderInstanceInput{
		ID: "anthropic", KindID: "anthropic", Label: "Anthropic",
		Config:  map[string]string{"baseUrl": "https://api.anthropic.com"},
		Secrets: map[string]string{"key": "sk-live-xyz"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// No stray .tmp file should survive a successful persist.
	if _, err := os.Stat(filepath.Join(dir, "providers.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected no leftover .tmp file, stat err=%v", err)
	}

	reopened, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	inst, ok := reopened.Get("anthropic")
	if !ok {
		t.Fatal("expected instance to survive reopen")
	}
	if inst.Config["baseUrl"] != "https://api.anthropic.com" {
		t.Fatalf("config not persisted: %+v", inst.Config)
	}
	if reopened.Secret("anthropic", "key") != "sk-live-xyz" {
		t.Fatalf("secret not persisted correctly: %q", reopened.Secret("anthropic", "key"))
	}
}
