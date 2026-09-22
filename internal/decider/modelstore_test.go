package decider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelStoreCreateUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	s := OpenModelStore(dir, fakeBox{}, nil)
	if s.Existed() || len(s.List()) != 0 {
		t.Fatal("a fresh store is not empty")
	}
	m, err := s.Upsert(ModelInput{Backend: SystemOneBackendID, Enabled: true, BaseURL: "http://127.0.0.1:8080/v1/", Secrets: map[string]string{SecretAPIKey: "k1"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "DM1" || m.Label != (systemOneBackend{}).Manifest().Label || m.Model != JevNativeModel || m.Credentials != CredentialsOwn {
		t.Errorf("defaults not filled: %+v", m)
	}
	if m.BaseURL != "http://127.0.0.1:8080/v1" || m.TimeoutMs != 3000 {
		t.Errorf("base URL / timeout = %q / %d", m.BaseURL, m.TimeoutMs)
	}
	if s.Secret(m.ID, SecretAPIKey) != "k1" || m.SecretsEnc[SecretAPIKey] == "k1" {
		t.Error("the key is not sealed, or cannot be opened")
	}
	if dto := m.ToDTO(); !dto.SecretsSet[SecretAPIKey] {
		t.Errorf("dto = %+v", dto)
	}

	// Update without secrets keeps the key; the backend cannot change.
	upd, err := s.Upsert(ModelInput{ID: m.ID, Backend: m.Backend, Label: "Local", Enabled: false, Model: OpenJevModel})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Label != "Local" || upd.Enabled || s.Secret(m.ID, SecretAPIKey) != "k1" || !upd.UpdatedAt.After(m.CreatedAt.Add(-1)) {
		t.Errorf("update = %+v", upd)
	}
	if _, err := s.Upsert(ModelInput{ID: m.ID, Backend: LogprobsBackendID, Model: "x"}); err == nil {
		t.Error("the backend of a model changed")
	}
	// An empty secret clears it.
	if _, err := s.Upsert(ModelInput{ID: m.ID, Backend: m.Backend, Model: OpenJevModel, Secrets: map[string]string{SecretAPIKey: ""}}); err != nil {
		t.Fatal(err)
	}
	if s.Secret(m.ID, SecretAPIKey) != "" {
		t.Error("the key survived an empty secret")
	}

	second, _ := s.Upsert(ModelInput{Backend: LogprobsBackendID, Model: "qwen3:4b"})
	if second.ID != "DM2" {
		t.Errorf("second id = %q", second.ID)
	}
	named, err := s.Upsert(ModelInput{ID: "jev-cloud", Backend: OpenRouterBackendID, Credentials: CredentialsProvider})
	if err != nil || named.ID != "jev-cloud" {
		t.Errorf("named model = %+v, %v", named, err)
	}

	again := OpenModelStore(dir, fakeBox{}, nil)
	if !again.Existed() || len(again.List()) != 3 || again.Secret("DM1", SecretAPIKey) != "" {
		t.Errorf("reloaded = %+v", again.List())
	}
	if err := again.Delete("DM2"); err != nil {
		t.Fatal(err)
	}
	if err := again.Delete("DM2"); err == nil {
		t.Error("deleting a missing model succeeded")
	}
	if len(OpenModelStore(dir, fakeBox{}, nil).List()) != 2 {
		t.Error("delete not persisted")
	}
}

func TestModelInputValidation(t *testing.T) {
	s := OpenModelStore("", fakeBox{}, nil)
	cases := []struct {
		name string
		in   ModelInput
		want string
	}{
		{"unknown backend", ModelInput{Backend: "nope"}, "unknown decision backend"},
		{"key required", ModelInput{Backend: OpenRouterBackendID, Credentials: CredentialsOwn}, "needs an API key"},
		{"bad credentials", ModelInput{Backend: OpenRouterBackendID, Credentials: "stolen"}, "unknown credential source"},
		{"bad base URL", ModelInput{Backend: SystemOneBackendID, BaseURL: "ftp://x"}, "must start with http"},
		{"unknown field", ModelInput{Backend: SystemOneBackendID, Config: map[string]string{"temperature": "1"}}, "unknown field"},
		{"unknown secret", ModelInput{Backend: SystemOneBackendID, Secrets: map[string]string{"password": "x"}}, "unknown secret"},
		{"bad extra body", ModelInput{Backend: LogprobsBackendID, Model: "m", Config: map[string]string{fieldExtraBody: "[1]"}}, "JSON object"},
		{"bad number", ModelInput{Backend: LogprobsBackendID, Model: "m", Config: map[string]string{fieldTopLogprobs: "50"}}, "topLogprobs"},
		{"bad id", ModelInput{ID: "has space", Backend: SystemOneBackendID}, "invalid decision model id"},
	}
	for _, c := range cases {
		_, err := s.Upsert(c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.want)
		}
	}
	// Borrowed credentials never keep an own base URL: the provider's key only
	// goes to the provider's endpoint.
	m, err := s.Upsert(ModelInput{Backend: OpenRouterBackendID, Credentials: CredentialsProvider, BaseURL: "https://evil.example/v1", ProviderInstanceID: " PRV1 "})
	if err != nil {
		t.Fatal(err)
	}
	if m.BaseURL != "" || m.ProviderInstanceID != "PRV1" {
		t.Errorf("borrowing model = %+v", m)
	}
	// Timeouts and context sizes are clamped.
	m, _ = s.Upsert(ModelInput{Backend: SystemOneBackendID, TimeoutMs: 999_999, ContextTokens: 10})
	if m.TimeoutMs != maxTimeoutMs || m.ContextTokens != minContextTokens {
		t.Errorf("clamps = %d / %d", m.TimeoutMs, m.ContextTokens)
	}
	// Without a secret box a key cannot be stored.
	if _, err := OpenModelStore("", nil, nil).Upsert(ModelInput{Backend: SystemOneBackendID, Secrets: map[string]string{SecretAPIKey: "k"}}); err == nil {
		t.Error("a key was stored without a secret box")
	}
}

func TestModelStoreMovesCorruptFileAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, modelsFileName)
	if err := os.WriteFile(path, []byte("[{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := OpenModelStore(dir, fakeBox{}, nil)
	if !s.Existed() || len(s.List()) != 0 {
		t.Errorf("corrupt store = %+v (existed=%v)", s.List(), s.Existed())
	}
	matches, _ := filepath.Glob(path + ".corrupt-*")
	if len(matches) != 1 {
		t.Errorf("corrupt file not kept aside: %v", matches)
	}
}
