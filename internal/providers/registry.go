package providers

import (
	"fmt"
	"os/exec"
)

// Registry builds providers by name using configured credentials and
// locally-available CLI tools.
type Registry struct {
	anthropicKey  string
	claudeCLIPath string // resolved path to `claude` binary, or "" if absent
}

// NewRegistry creates a registry. It auto-detects the claude CLI on PATH.
func NewRegistry(anthropicKey string) *Registry {
	path, _ := exec.LookPath("claude")
	return &Registry{
		anthropicKey:  anthropicKey,
		claudeCLIPath: path,
	}
}

// ClaudeCLIAvailable reports whether the claude CLI was found.
func (r *Registry) ClaudeCLIAvailable() bool { return r.claudeCLIPath != "" }

// Get returns a provider for the given name, or an error if unsupported
// or unconfigured.
func (r *Registry) Get(name string) (Provider, error) {
	switch name {
	case "anthropic":
		if r.anthropicKey == "" {
			return nil, fmt.Errorf("anthropic provider not configured (set ANTHROPIC_API_KEY)")
		}
		return NewAnthropic(r.anthropicKey), nil

	case "claude-cli", "":
		if r.claudeCLIPath == "" {
			return nil, fmt.Errorf("claude CLI not found on PATH (install Claude Code)")
		}
		return NewClaudeCLI(r.claudeCLIPath, ""), nil

	default:
		return nil, fmt.Errorf("unknown provider: %q", name)
	}
}
