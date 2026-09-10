package conversation

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// fingerprintHexLen is how many hex characters of the SHA-256 digest a
// fingerprint keeps: 16 (64 bits) is far beyond collision risk for the handful of
// catalogs / prefixes one workspace ever sees and keeps the calibration keys short.
const fingerprintHexLen = 16

// CatalogFingerprint identifies a shipped tool catalog by CONTENT: every tool's
// name, description and JSON input schema, in name order so the caller's
// ordering never changes the key. Two catalogs with the same fingerprint cost
// the model the same tokens, which is what the learned CLI-harness overhead is
// keyed on (a tool added, removed or re-described starts a new measurement).
func CatalogFingerprint(defs []providers.ToolDef) string {
	sorted := make([]providers.ToolDef, len(defs))
	copy(sorted, defs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	for _, d := range sorted {
		writeFramed(h, d.Name)
		writeFramed(h, d.Description)
		writeFramed(h, string(d.InputSchema))
	}
	return hex.EncodeToString(h.Sum(nil))[:fingerprintHexLen]
}

// PrefixFingerprint identifies a static prompt prefix — the system text plus the
// tool catalog — for exact-count caching. The model is part of the identity
// because the count comes from that model's tokenizer.
func PrefixFingerprint(model, system string, defs []providers.ToolDef) string {
	h := sha256.New()
	writeFramed(h, model)
	writeFramed(h, system)
	writeFramed(h, CatalogFingerprint(defs))
	return hex.EncodeToString(h.Sum(nil))[:fingerprintHexLen]
}

// writeFramed hashes s with a length prefix so adjacent fields cannot run into
// each other ("ab"+"c" must not equal "a"+"bc").
func writeFramed(h interface{ Write([]byte) (int, error) }, s string) {
	var lenBuf [8]byte
	n := len(s)
	for i := 7; i >= 0; i-- {
		lenBuf[i] = byte(n)
		n >>= 8
	}
	h.Write(lenBuf[:])
	h.Write([]byte(s))
}
