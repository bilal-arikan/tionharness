package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestExportLegacyKeysToOpenCode: keys of instances whose kind no longer
// exists land in the opencode auth store once; stored vendors are kept.
func TestExportLegacyKeysToOpenCode(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenProviderStore(dataDir, testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []ProviderInstanceInput{
		{ID: "anthropic", KindID: "anthropic", Enabled: true, Secrets: map[string]string{"key": "sk-ant-1"}},
		{ID: "openrouter", KindID: "openai-compat", Enabled: true, Config: map[string]string{"baseUrl": "https://openrouter.ai/api/v1"}, Secrets: map[string]string{"key": "sk-or-1"}},
		{ID: "deepseek", KindID: "deepseek", Enabled: true, Secrets: map[string]string{"key": "sk-ds-1"}},
		{ID: "deepseek-anthropic", KindID: "deepseek-anthropic", Enabled: true, Secrets: map[string]string{"key": "sk-ds-2"}},
		{ID: "zai", KindID: "zai", Enabled: true},
		{ID: "claude-cli", KindID: "claude-cli", Enabled: true},
	} {
		if _, err := store.Upsert(in); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dataDir, "opencode-home", "data", "opencode", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"deepseek":{"type":"api","key":"mine"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	registered := func(k string) bool { return k == "claude-cli" }

	added, err := store.ExportLegacyKeysToOpenCode(dataDir, registered)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"anthropic", "openrouter"}; len(added) != 2 || added[0] != want[0] || added[1] != want[1] {
		t.Fatalf("added = %v, want %v", added, want)
	}
	raw, _ := os.ReadFile(path)
	var got map[string]openCodeAuthEntry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["anthropic"].Key != "sk-ant-1" || got["anthropic"].Type != "api" || got["openrouter"].Key != "sk-or-1" {
		t.Errorf("entries = %+v", got)
	}
	if got["deepseek"].Key != "mine" {
		t.Errorf("an existing vendor credential was overwritten: %+v", got["deepseek"])
	}
	if _, ok := got["zai"]; ok {
		t.Error("a keyless instance must not produce an entry")
	}
	if again, err := store.ExportLegacyKeysToOpenCode(dataDir, registered); err != nil || again != nil {
		t.Errorf("second run: added=%v err=%v", again, err)
	}
}
