package repair

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// CallKey identifies one tool call by name + input hash: the unit the repair
// guard, the loop guardrail and the dead-tool repair all key their per-turn
// memory on.
// callKey collapses a tool call to a stable identity: name + input digest.
func CallKey(call providers.ToolCall) string {
	sum := sha256.Sum256(call.Input)
	return call.Name + ":" + hex.EncodeToString(sum[:8])
}
