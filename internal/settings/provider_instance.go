package settings

import "time"

// ProviderInstance is a user-configured provider: a kind (the built-in
// behaviour it runs — "anthropic", "claude-cli", "openai-compat", ...) plus a
// label and the concrete fields that kind needs (open config + encrypted
// secrets). Unlike a ProviderKind, instances are created and deleted by users
// and there can be many of the same kind (e.g. two "anthropic" instances with
// different API keys). SecretsEnc is never serialized to the API (json tag
// "-"); DTO exposes only which secret keys are set.
type ProviderInstance struct {
	ID           string            `json:"id"`
	KindID       string            `json:"kindId"`
	Label        string            `json:"label"`
	Icon         string            `json:"icon"`
	Enabled      bool              `json:"enabled"`
	DefaultModel string            `json:"defaultModel"`
	Models       string            `json:"models"`
	Config       map[string]string `json:"config"`
	SecretsEnc   map[string]string `json:"-"`
	CreatedAt    time.Time         `json:"createdAt"`
}

// providerInstanceFile is the on-disk shape of a ProviderInstance: identical
// to the API-facing struct but WITH the encrypted secrets, since json:"-" on
// ProviderInstance.SecretsEnc hides it from json.Marshal entirely (the tag
// exists precisely so an accidental ToDTO()-skip can never leak it to the
// API). Persistence goes through this mirror struct instead.
type providerInstanceFile struct {
	ID           string            `json:"id"`
	KindID       string            `json:"kindId"`
	Label        string            `json:"label"`
	Icon         string            `json:"icon"`
	Enabled      bool              `json:"enabled"`
	DefaultModel string            `json:"defaultModel"`
	Models       string            `json:"models"`
	Config       map[string]string `json:"config"`
	SecretsEnc   map[string]string `json:"secretsEnc"`
	CreatedAt    time.Time         `json:"createdAt"`
}

func (p ProviderInstance) toFile() providerInstanceFile {
	return providerInstanceFile{
		ID:           p.ID,
		KindID:       p.KindID,
		Label:        p.Label,
		Icon:         p.Icon,
		Enabled:      p.Enabled,
		DefaultModel: p.DefaultModel,
		Models:       p.Models,
		Config:       p.Config,
		SecretsEnc:   p.SecretsEnc,
		CreatedAt:    p.CreatedAt,
	}
}

func (f providerInstanceFile) toInstance() ProviderInstance {
	return ProviderInstance{
		ID:           f.ID,
		KindID:       f.KindID,
		Label:        f.Label,
		Icon:         f.Icon,
		Enabled:      f.Enabled,
		DefaultModel: f.DefaultModel,
		Models:       f.Models,
		Config:       f.Config,
		SecretsEnc:   f.SecretsEnc,
		CreatedAt:    f.CreatedAt,
	}
}

// ProviderInstanceDTO is the masked, client-facing view of a ProviderInstance:
// identical minus the encrypted secrets, plus a per-key "is it set" map.
type ProviderInstanceDTO struct {
	ID           string            `json:"id"`
	KindID       string            `json:"kindId"`
	Label        string            `json:"label"`
	Icon         string            `json:"icon"`
	Enabled      bool              `json:"enabled"`
	DefaultModel string            `json:"defaultModel"`
	Models       string            `json:"models"`
	Config       map[string]string `json:"config"`
	SecretsSet   map[string]bool   `json:"secretsSet"`
	CreatedAt    time.Time         `json:"createdAt"`
}

// ProviderInstanceInput is the upsert request shape. Secrets is write-only
// plaintext, following the same convention as Patch's write-only key fields
// (see settings.go): a key present with an empty value CLEARS the stored
// secret, a key present with a non-empty value REPLACES it, and a key that is
// simply absent from the map LEAVES the existing stored secret untouched. This
// lets a client update the label or config without having to resend (or blow
// away) an already-configured API key.
type ProviderInstanceInput struct {
	ID           string            `json:"id"`
	KindID       string            `json:"kindId"`
	Label        string            `json:"label"`
	Icon         string            `json:"icon"`
	Enabled      bool              `json:"enabled"`
	DefaultModel string            `json:"defaultModel"`
	Models       string            `json:"models"`
	Config       map[string]string `json:"config"`
	Secrets      map[string]string `json:"secrets"`
}

// ToDTO projects a ProviderInstance into its masked client-facing view.
func (p ProviderInstance) ToDTO() ProviderInstanceDTO {
	set := make(map[string]bool, len(p.SecretsEnc))
	for k, enc := range p.SecretsEnc {
		set[k] = enc != ""
	}
	return ProviderInstanceDTO{
		ID:           p.ID,
		KindID:       p.KindID,
		Label:        p.Label,
		Icon:         p.Icon,
		Enabled:      p.Enabled,
		DefaultModel: p.DefaultModel,
		Models:       p.Models,
		Config:       copyStringMap(p.Config),
		SecretsSet:   set,
		CreatedAt:    p.CreatedAt,
	}
}

// copyStringMap returns a defensive shallow copy so callers can't mutate the
// stored instance's map through a DTO or a List() result.
func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
