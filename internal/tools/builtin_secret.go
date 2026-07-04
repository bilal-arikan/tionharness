package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/secrets"
)

// SecretTool is the single entry point for the workspace secret vault: it lists,
// reads, stores and removes secrets (API keys, tokens, passwords) via an `action`
// discriminator. It replaces the former one-per-verb tools (secret_list /
// secret_get / secret_set / secret_delete). Values are AES-GCM encrypted at rest.
//
// The whole tool is classified RiskWrite (its default), so it is blocked in
// read-only mode and needs approval in "ask" mode — the same gating the write
// verbs already carried; the read verbs were never read-only-safe either, so no
// capability is lost by merging them under one risk tier.
type SecretTool struct{ vault *secrets.Vault }

// NewSecretTool binds the tool to a workspace vault.
func NewSecretTool(vault *secrets.Vault) SecretTool { return SecretTool{vault: vault} }

func (SecretTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "secret",
		Description: "Manage this workspace's encrypted secret vault (API keys, tokens, passwords). " +
			"Set `action`: `list` (names + descriptions, no values), `get` (the decrypted value of one " +
			"secret by name — use it to perform the task, do not echo it to the user unless asked), " +
			"`set` (store or overwrite a secret; requires name + value, optional description), or " +
			"`delete` (remove one by name). Values are encrypted at rest.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"action":{"type":"string","enum":["list","get","set","delete"],"description":"What to do with the vault"},
				"name":{"type":"string","description":"Secret name (required for get/set/delete)"},
				"value":{"type":"string","description":"Secret value to store (required for set; encrypted at rest)"},
				"description":{"type":"string","description":"Optional human description for set (no value leaked)"}
			},
			"required":["action"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"action":"list"}`),
			json.RawMessage(`{"action":"get","name":"OPENAI_API_KEY"}`),
			json.RawMessage(`{"action":"set","name":"OPENAI_API_KEY","value":"sk-...","description":"OpenAI key for the summarizer"}`),
			json.RawMessage(`{"action":"delete","name":"OPENAI_API_KEY"}`),
		},
	}
}

func (t SecretTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.vault == nil {
		return "", fmt.Errorf("no secret vault is available in this workspace")
	}
	var in struct {
		Action      string `json:"action"`
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("secret", err)
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	name := strings.TrimSpace(in.Name)

	switch action {
	case "list":
		return t.list(), nil
	case "get":
		if name == "" {
			return "", fmt.Errorf("name is required for action \"get\"")
		}
		value, ok := t.vault.Get(name)
		if !ok {
			return fmt.Sprintf("No secret named %q exists in this workspace. Use action \"list\" to see available names.", name), nil
		}
		return value, nil
	case "set":
		if name == "" {
			return "", fmt.Errorf("name is required for action \"set\"")
		}
		if in.Value == "" {
			return "", fmt.Errorf("value is required for action \"set\"")
		}
		if _, err := t.vault.Set(name, in.Value, in.Description); err != nil {
			return "", fmt.Errorf("set secret: %w", err)
		}
		b, _ := json.Marshal(map[string]string{"name": name, "action": "stored"})
		return string(b), nil
	case "delete":
		if name == "" {
			return "", fmt.Errorf("name is required for action \"delete\"")
		}
		if err := t.vault.Delete(name); err != nil {
			return "", fmt.Errorf("delete secret: %w", err)
		}
		b, _ := json.Marshal(map[string]string{"name": name, "action": "deleted"})
		return string(b), nil
	default:
		return "", fmt.Errorf("unknown action %q (use list/get/set/delete)", in.Action)
	}
}

// list renders the vault's secret names + descriptions (never values).
func (t SecretTool) list() string {
	metas := t.vault.Names()
	if len(metas) == 0 {
		return "No secrets are stored in this workspace."
	}
	var b strings.Builder
	b.WriteString("Available secrets (use action \"get\" with the name):\n")
	for _, m := range metas {
		if strings.TrimSpace(m.Description) != "" {
			fmt.Fprintf(&b, "- %s — %s\n", m.Name, m.Description)
		} else {
			fmt.Fprintf(&b, "- %s\n", m.Name)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
