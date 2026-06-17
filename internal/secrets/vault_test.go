package secrets

import (
	"strings"
	"testing"
)

// stubCipher is a reversible non-crypto cipher for tests (prefix tag), so the
// vault logic can be verified without a real AES key.
type stubCipher struct{}

func (stubCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (stubCipher) Decrypt(c string) (string, error) { return strings.TrimPrefix(c, "enc:"), nil }

func TestVaultRoundTrip(t *testing.T) {
	dir := t.TempDir()

	v, err := Open(dir, stubCipher{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, err := v.Set("API_KEY", "s3cr3t", "Service key"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := v.Set("DB_PASS", "hunter2", ""); err != nil {
		t.Fatalf("set: %v", err)
	}

	// List is masked and sorted.
	list := v.List()
	if len(list) != 2 || list[0].Name != "API_KEY" || list[1].Name != "DB_PASS" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Get decrypts.
	if val, ok := v.Get("API_KEY"); !ok || val != "s3cr3t" {
		t.Fatalf("get API_KEY = %q,%v", val, ok)
	}

	// Reopen from disk: values survive and decrypt.
	v2, err := Open(dir, stubCipher{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if val, ok := v2.Get("DB_PASS"); !ok || val != "hunter2" {
		t.Fatalf("reopened get DB_PASS = %q,%v", val, ok)
	}
	if len(v2.List()) != 2 {
		t.Fatalf("reopened list len = %d", len(v2.List()))
	}

	// Update keeps CreatedAt, refreshes value.
	before := v2.List()[0]
	if _, err := v2.Set("API_KEY", "rotated", "Service key"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	after, _ := v2.Get("API_KEY")
	if after != "rotated" {
		t.Fatalf("rotate value = %q", after)
	}
	if v2.List()[0].CreatedAt != before.CreatedAt {
		t.Fatalf("CreatedAt changed on update")
	}

	// Delete.
	if err := v2.Delete("DB_PASS"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := v2.Get("DB_PASS"); ok {
		t.Fatalf("deleted secret still present")
	}
	if err := v2.Delete("DB_PASS"); err != ErrNotFound {
		t.Fatalf("delete absent = %v, want ErrNotFound", err)
	}
}

func TestVaultValidation(t *testing.T) {
	v, _ := Open(t.TempDir(), stubCipher{})

	bad := []string{"", "1leading", "has space", "uniçode", "a/b"}
	for _, n := range bad {
		if _, err := v.Set(n, "x", ""); err != ErrInvalidName {
			t.Errorf("Set(%q) err = %v, want ErrInvalidName", n, err)
		}
	}
	good := []string{"A", "API_KEY", "db-pass", "x.y.z", "Token1"}
	for _, n := range good {
		if _, err := v.Set(n, "x", ""); err != nil {
			t.Errorf("Set(%q) err = %v, want nil", n, err)
		}
	}

	// Empty value rejected.
	if _, err := v.Set("OK", "", ""); err != ErrInvalidValue {
		t.Errorf("empty value err = %v, want ErrInvalidValue", err)
	}
}
