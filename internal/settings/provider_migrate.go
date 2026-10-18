package settings

// defaultProviderInstances is the instance set a fresh install starts with: the
// two keyless CLI transports, each running against its app-global config home.
// An instance's ID equals its kind ID ("claude-cli", "codex-cli") so
// Agent.Provider values, which hold a kind id, resolve with no per-agent
// migration (_Docs/71 §3). It used to be derived from legacy settings.json
// fields (CLI paths, config dirs, a claude-cli token); those were removed on
// 2026-10-18 once every install had a providers.json.
func defaultProviderInstances() []ProviderInstance {
	now := currentTime()
	return []ProviderInstance{
		{
			ID:         "claude-cli",
			KindID:     "claude-cli",
			Label:      "Claude CLI",
			Enabled:    true,
			Config:     map[string]string{"configDir": defaultClaudeConfigDir()},
			SecretsEnc: map[string]string{},
			CreatedAt:  now,
		},
		{
			ID:         "codex-cli",
			KindID:     "codex-cli",
			Label:      "Codex CLI",
			Enabled:    true,
			Config:     map[string]string{},
			SecretsEnc: map[string]string{},
			CreatedAt:  now,
		},
	}
}

// EnsureSeeded writes defaultProviderInstances, but ONLY if providers.json did
// NOT already exist on disk when this ProviderStore was opened (s.fileExisted
// false). It never overwrites an existing file — including one
// OpenProviderStore itself just created as an empty `[]`, since that already
// counts as "existed" for the next call on the same store. Returns whether it
// seeded.
func (s *ProviderStore) EnsureSeeded() (bool, error) {
	s.mu.Lock()
	if s.fileExisted {
		s.mu.Unlock()
		return false, nil
	}
	s.fileExisted = true // claim the run before releasing the lock
	s.mu.Unlock()

	instances := defaultProviderInstances()

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persist(instances); err != nil {
		return false, err
	}
	s.cur = instances
	return true, nil
}

// EnsureKindInstance adds a default instance for kindID (id == kindID, like
// the migrated built-ins) when providers.json holds no instance of that kind
// yet. It runs on every boot — unlike EnsureSeeded, which is one-shot — so a
// keyless CLI transport added after the first boot (opencode-cli, _Docs/95)
// still shows up for an existing install once its binary is found. A kind the
// user deliberately removed comes back only if they removed every instance of
// it, which is the same contract claude-cli has had since migration. Returns
// whether an instance was added.
func (s *ProviderStore) EnsureKindInstance(kindID, label string) (bool, error) {
	s.mu.Lock()
	for _, p := range s.cur {
		if p.KindID == kindID {
			s.mu.Unlock()
			return false, nil
		}
	}
	s.mu.Unlock()
	_, err := s.Upsert(ProviderInstanceInput{ID: kindID, KindID: kindID, Label: label, Enabled: true, Config: map[string]string{}})
	if err != nil {
		return false, err
	}
	return true, nil
}
