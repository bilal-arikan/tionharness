package settings

// Cipher returns the cipher that seals provider secrets. The decision-model
// layer (internal/decider) seals the API keys its models carry with the same
// one, so every stored credential in the data directory is protected alike.
func (s *ProviderStore) Cipher() Cipher { return s.cipher }
