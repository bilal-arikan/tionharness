package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/secrets"
)

// SecretListTool lets an agent discover which secrets exist in its workspace.
// It returns only names and descriptions — never the values.
type SecretListTool struct {
	vault *secrets.Vault
}

// NewSecretListTool binds the tool to a workspace vault.
func NewSecretListTool(vault *secrets.Vault) SecretListTool {
	return SecretListTool{vault: vault}
}

func (SecretListTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "secret_list",
		Description: "List the names and descriptions of secrets (API keys, tokens, passwords) stored in this workspace's vault. Values are NOT returned — use secret_get to fetch one by name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t SecretListTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	if t.vault == nil {
		return "No secret vault is available in this workspace.", nil
	}
	metas := t.vault.Names()
	if len(metas) == 0 {
		return "No secrets are stored in this workspace.", nil
	}
	var b strings.Builder
	b.WriteString("Available secrets (use secret_get with the name):\n")
	for _, m := range metas {
		if strings.TrimSpace(m.Description) != "" {
			fmt.Fprintf(&b, "- %s — %s\n", m.Name, m.Description)
		} else {
			fmt.Fprintf(&b, "- %s\n", m.Name)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// SecretGetTool returns the decrypted value of a named secret so the agent can
// use it (e.g. authenticate to an API). The value is sensitive — it should be
// used in the requested action, not echoed back to the user verbatim.
type SecretGetTool struct {
	vault *secrets.Vault
}

// NewSecretGetTool binds the tool to a workspace vault.
func NewSecretGetTool(vault *secrets.Vault) SecretGetTool {
	return SecretGetTool{vault: vault}
}

func (SecretGetTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "secret_get",
		Description: "Fetch the value of a secret (API key, token, password) from this workspace's vault by its exact name. Use this to obtain credentials needed for a task. Do not reveal the raw value to the user unless explicitly asked; use it to perform the requested action.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"The exact secret name (see secret_list)"}
			},
			"required":["name"],
			"additionalProperties":false
		}`),
	}
}

func (t SecretGetTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if t.vault == nil {
		return "", fmt.Errorf("no secret vault is available in this workspace")
	}
	value, ok := t.vault.Get(name)
	if !ok {
		return fmt.Sprintf("No secret named %q exists in this workspace. Use secret_list to see available names.", name), nil
	}
	return value, nil
}
