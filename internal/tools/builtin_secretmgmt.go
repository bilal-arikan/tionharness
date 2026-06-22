package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/secrets"
)

// Secret write tools let an agent store and remove secrets (API keys, tokens)
// in its workspace vault — the write complement to secret_list / secret_get. An
// agent can thus capture a credential it obtains (e.g. to wire up an MCP server
// or webhook) for later reuse. Values are AES-GCM encrypted at rest by the vault.

// ---- secret_set ----

// SecretSetTool writes (creates or updates) a secret value.
type SecretSetTool struct{ vault *secrets.Vault }

// NewSecretSetTool binds the tool to a workspace vault.
func NewSecretSetTool(vault *secrets.Vault) SecretSetTool { return SecretSetTool{vault: vault} }

func (SecretSetTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "secret_set",
		Description: "Store a secret (API key, token, password) in this workspace's encrypted vault, creating it or overwriting an existing one with the same name. Use secret_get to read it back later. Returns the stored name.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Secret name (identifier used with secret_get)"},
				"value":{"type":"string","description":"The secret value to store (encrypted at rest)"},
				"description":{"type":"string","description":"Optional human description (no value leaked)"}
			},
			"required":["name","value"],
			"additionalProperties":false
		}`),
	}
}

func (t SecretSetTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.vault == nil {
		return "", fmt.Errorf("no secret vault is available in this workspace")
	}
	var in struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if in.Value == "" {
		return "", fmt.Errorf("value is required")
	}
	if _, err := t.vault.Set(in.Name, in.Value, in.Description); err != nil {
		return "", fmt.Errorf("set secret: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"name": in.Name, "action": "stored"})
	return string(b), nil
}

// ---- secret_delete ----

// SecretDeleteTool removes a secret from the vault.
type SecretDeleteTool struct{ vault *secrets.Vault }

// NewSecretDeleteTool binds the tool to a workspace vault.
func NewSecretDeleteTool(vault *secrets.Vault) SecretDeleteTool {
	return SecretDeleteTool{vault: vault}
}

func (SecretDeleteTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "secret_delete",
		Description: "Delete a secret from this workspace's vault by name.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"name":{"type":"string","description":"The secret name (see secret_list)"}},
			"required":["name"],
			"additionalProperties":false
		}`),
	}
}

func (t SecretDeleteTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.vault == nil {
		return "", fmt.Errorf("no secret vault is available in this workspace")
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := t.vault.Delete(in.Name); err != nil {
		return "", fmt.Errorf("delete secret: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"name": in.Name, "action": "deleted"})
	return string(b), nil
}
