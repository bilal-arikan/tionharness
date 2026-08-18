package settings

import "strings"

// MigrateFromSettings derives the provider-instance set implied by a legacy
// Settings document, per the mapping table in _Docs/71-SAGLAYICI-ORNEKLERI-PLANI.md
// §3. decrypt opens an *Enc field to plaintext (return "" on failure — this
// function never fails the boot sequence over an undecryptable legacy secret,
// it just migrates that provider without a working key).
//
// The key invariant this preserves: a migrated instance's ID equals its kind
// ID ("anthropic", "claude-cli", ...) so existing Agent.Provider values (which
// already hold a kind id) resolve to the right instance with no separate
// per-agent migration pass (_Docs/71 §3).
func MigrateFromSettings(s Settings, decrypt func(enc string) string) []ProviderInstance {
	var out []ProviderInstance
	now := currentTime()

	if s.AnthropicKeyEnc != "" {
		out = append(out, ProviderInstance{
			ID:         "anthropic",
			KindID:     "anthropic",
			Label:      "Anthropic",
			Enabled:    true,
			Config:     map[string]string{},
			SecretsEnc: map[string]string{"key": s.AnthropicKeyEnc},
			CreatedAt:  now,
		})
	}

	// claude-cli migrates unconditionally: it runs keyless against the ambient
	// or configured CLI login, so "no fields set" is still a valid, usable
	// instance (_Docs/71 §3).
	{
		cfg := map[string]string{
			"cliPath":   s.ClaudeCLIPath,
			"configDir": s.ClaudeConfigDir,
			"authKind":  s.ClaudeCliAuthKind,
		}
		secrets := map[string]string{}
		if s.ClaudeCliAuthTokenEnc != "" {
			secrets["authToken"] = s.ClaudeCliAuthTokenEnc
		}
		out = append(out, ProviderInstance{
			ID:         "claude-cli",
			KindID:     "claude-cli",
			Label:      "Claude CLI",
			Enabled:    true,
			Config:     cfg,
			SecretsEnc: secrets,
			CreatedAt:  now,
		})
	}

	if strings.TrimSpace(s.CodexCLIPath) != "" || strings.TrimSpace(s.CodexConfigDir) != "" {
		out = append(out, ProviderInstance{
			ID:      "codex-cli",
			KindID:  "codex-cli",
			Label:   "Codex CLI",
			Enabled: true,
			Config: map[string]string{
				"cliPath":   s.CodexCLIPath,
				"configDir": s.CodexConfigDir,
			},
			SecretsEnc: map[string]string{},
			CreatedAt:  now,
		})
	}

	if s.MinimaxKeyEnc != "" {
		out = append(out, ProviderInstance{
			ID:         "minimax",
			KindID:     "minimax",
			Label:      "MiniMax",
			Enabled:    true,
			Config:     map[string]string{"baseUrl": s.MinimaxBaseURL},
			SecretsEnc: map[string]string{"key": s.MinimaxKeyEnc},
			CreatedAt:  now,
		})
	}

	if s.OpenRouterKeyEnc != "" {
		out = append(out, ProviderInstance{
			ID:         "openrouter",
			KindID:     "openrouter",
			Label:      "OpenRouter",
			Enabled:    true,
			Config:     map[string]string{"baseUrl": s.OpenRouterBaseURL},
			SecretsEnc: map[string]string{"key": s.OpenRouterKeyEnc},
			CreatedAt:  now,
		})
	}

	if s.ZAIKeyEnc != "" {
		out = append(out, ProviderInstance{
			ID:         "zai",
			KindID:     "zai",
			Label:      "Z.ai GLM",
			Enabled:    true,
			Config:     map[string]string{"baseUrl": s.ZAIBaseURL},
			SecretsEnc: map[string]string{"key": s.ZAIKeyEnc},
			CreatedAt:  now,
		})
	}

	if s.DeepSeekKeyEnc != "" {
		out = append(out, ProviderInstance{
			ID:         "deepseek",
			KindID:     "deepseek",
			Label:      "DeepSeek",
			Enabled:    true,
			Config:     map[string]string{"baseUrl": s.DeepSeekBaseURL},
			SecretsEnc: map[string]string{"key": s.DeepSeekKeyEnc},
			CreatedAt:  now,
		})
	}

	for _, c := range s.CustomProviders {
		kindID := "openai-compat"
		if c.Kind == "anthropic" {
			kindID = "anthropic-compat"
		}
		secrets := map[string]string{}
		if c.KeyEnc != "" {
			secrets["key"] = c.KeyEnc
		}
		out = append(out, ProviderInstance{
			ID:           c.ID,
			KindID:       kindID,
			Label:        c.Label,
			Enabled:      true,
			DefaultModel: c.DefaultModel,
			Models:       c.Models,
			Config:       map[string]string{"baseUrl": c.BaseURL},
			SecretsEnc:   secrets,
			CreatedAt:    now,
		})
	}

	// _ silences decrypt-unused when no *Enc field above ever needed live
	// plaintext — every field here migrates its *Enc value as-is (still
	// AES-GCM sealed under the same cipher), so decrypt is reserved for
	// callers that need to re-derive a value rather than just relocate it.
	// Kept as a parameter (rather than dropped) so a future field that DOES
	// need plaintext-at-migration-time (e.g. re-deriving a default label from
	// a decrypted value) doesn't require an API change.
	_ = decrypt

	return out
}

// EnsureMigrated runs MigrateFromSettings and persists the result, but ONLY
// if providers.json did NOT already exist on disk when this ProviderStore
// was opened (s.fileExisted false). It never overwrites an existing file —
// including one OpenProviderStore itself just created as an empty `[]`,
// since that already counts as "existed" for the next EnsureMigrated call on
// the same store, and migration is a boot-time, run-at-most-once operation.
func (s *ProviderStore) EnsureMigrated(sett Settings, decrypt func(string) string) (bool, error) {
	s.mu.Lock()
	if s.fileExisted {
		s.mu.Unlock()
		return false, nil
	}
	s.fileExisted = true // claim the run before releasing the lock
	s.mu.Unlock()

	instances := MigrateFromSettings(sett, decrypt)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persist(instances); err != nil {
		return false, err
	}
	s.cur = instances
	return true, nil
}
