package settings

import "testing"

// plainCipher is a no-op cipher for tests (encryption is exercised elsewhere).
type plainCipher struct{}

func (plainCipher) Encrypt(s string) (string, error) { return "enc:" + s, nil }
func (plainCipher) Decrypt(s string) (string, error) {
	if len(s) >= 4 && s[:4] == "enc:" {
		return s[4:], nil
	}
	return "", nil
}

func TestCustomProviderCRUD(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, plainCipher{})
	if err != nil {
		t.Fatal(err)
	}

	// Reserved id is rejected.
	if _, err := store.UpsertCustomProvider(CustomProvider{ID: "anthropic", Kind: "openai", BaseURL: "x"}, nil); err == nil {
		t.Error("reserved id should be rejected")
	}
	// Bad kind / missing base URL rejected.
	if _, err := store.UpsertCustomProvider(CustomProvider{ID: "or", Kind: "bogus", BaseURL: "x"}, nil); err == nil {
		t.Error("bad kind should be rejected")
	}
	if _, err := store.UpsertCustomProvider(CustomProvider{ID: "or", Kind: "openai", BaseURL: ""}, nil); err == nil {
		t.Error("missing base URL should be rejected")
	}

	// Create with key.
	key := "sk-123"
	if _, err := store.UpsertCustomProvider(CustomProvider{ID: "myrouter", Label: "myrouter", Kind: "openai", BaseURL: "https://myrouter.ai/api/v1"}, &key); err != nil {
		t.Fatal(err)
	}
	if got := store.CustomProviderKey("myrouter"); got != key {
		t.Errorf("key = %q, want %q", got, key)
	}

	// Update without a key keeps the stored key.
	if _, err := store.UpsertCustomProvider(CustomProvider{ID: "myrouter", Label: "OR v2", Kind: "openai", BaseURL: "https://myrouter.ai/api/v1"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := store.CustomProviderKey("myrouter"); got != key {
		t.Errorf("key after meta update = %q, want preserved %q", got, key)
	}
	dto := store.DTO().CustomProviders
	if len(dto) != 1 || dto[0].Label != "OR v2" || !dto[0].KeySet {
		t.Fatalf("dto wrong: %+v", dto)
	}

	// Persists across reopen.
	store2, err := Open(dir, plainCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if store2.CustomProviderKey("myrouter") != key {
		t.Error("key did not persist across reopen")
	}

	// Delete.
	if _, err := store2.DeleteCustomProvider("myrouter"); err != nil {
		t.Fatal(err)
	}
	if len(store2.DTO().CustomProviders) != 0 {
		t.Error("provider not deleted")
	}
	if _, err := store2.DeleteCustomProvider("nope"); err == nil {
		t.Error("deleting missing provider should error")
	}
}

