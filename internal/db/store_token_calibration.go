package db

import (
	"context"
	"strings"
	"time"
)

// tokenCalibrationsFile is the singleton workspace document holding every
// learned token-accounting fact: measured CLI-harness overheads and exact
// prompt-prefix counts. It sits beside model-resolutions.json and follows the
// same "observe, remember, never hardcode" rule.
const tokenCalibrationsFile = "token-calibrations.json"

// Token calibration kinds.
const (
	// TokenCalibrationCLIOverhead is the MEASURED gap between TionHarness's own
	// segment estimate and the real prompt a CLI-wrapper provider (claude-cli)
	// sent to the model: the CLI's system prompt + built-in tools + MCP bridge.
	// Keyed by (provider, CLI version, shipped tool-catalog fingerprint) because
	// each of those changes the harness cost.
	TokenCalibrationCLIOverhead = "cli-overhead"
	// TokenCalibrationPrefix is the EXACT server-side token count of a static
	// prompt prefix (system prompt + tool schemas) for one model, obtained once
	// from the provider's count endpoint and reused until the prefix changes.
	TokenCalibrationPrefix = "prefix"
)

// tokenCalibrationWindow bounds the running mean of an OBSERVED calibration: the
// newest sample always carries at least 1/window of the weight, so a harness
// that grew (CLI update, new bridged tools) is tracked within a few turns
// instead of being averaged away by a long history.
const tokenCalibrationWindow = 8

// TokenCalibration is one learned token-accounting fact (see the kinds above).
type TokenCalibration struct {
	Key         string    `json:"key"`
	Kind        string    `json:"kind"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model,omitempty"`
	CLIVersion  string    `json:"cliVersion,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Tokens      int       `json:"tokens"`
	Samples     int       `json:"samples"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// CLIOverheadCalibrationKey names the learned harness overhead of one CLI
// provider version for one shipped tool catalog.
func CLIOverheadCalibrationKey(provider, cliVersion, catalogFingerprint string) string {
	return TokenCalibrationCLIOverhead + "|" + strings.TrimSpace(provider) + "|" + strings.TrimSpace(cliVersion) + "|" + catalogFingerprint
}

// PrefixCalibrationKey names the exact count of one prompt prefix for one model.
func PrefixCalibrationKey(provider, model, prefixFingerprint string) string {
	return TokenCalibrationPrefix + "|" + strings.TrimSpace(provider) + "|" + strings.TrimSpace(model) + "|" + prefixFingerprint
}

// TokenCalibration returns the learned fact stored under key, if any.
func (d *DB) TokenCalibration(key string) (TokenCalibration, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	c, ok := d.tokenCalibrations[key]
	return c, ok
}

// SetTokenCalibration stores an EXACT fact (a server-side count), replacing any
// earlier value under the same key. Samples is reset to 1: exact counts are not
// averaged. A no-op when the stored value already matches, so a hot path can
// call it freely.
func (d *DB) SetTokenCalibration(ctx context.Context, c TokenCalibration) error {
	if c.Key == "" {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if cur, ok := d.tokenCalibrations[c.Key]; ok && cur.Tokens == c.Tokens && cur.Samples >= 1 {
		return nil
	}
	c.Samples = 1
	c.UpdatedAt = time.Now()
	d.tokenCalibrations[c.Key] = c
	return atomicWriteJSON(d.dir(tokenCalibrationsFile), d.tokenCalibrations)
}

// ObserveTokenCalibration folds one MEASURED sample into the running mean stored
// under c.Key (c.Tokens is the sample). The mean is windowed to
// tokenCalibrationWindow samples so recent observations dominate. Returns the
// updated entry.
func (d *DB) ObserveTokenCalibration(ctx context.Context, c TokenCalibration) (TokenCalibration, error) {
	if c.Key == "" {
		return c, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.tokenCalibrations[c.Key]
	if !ok || cur.Samples <= 0 {
		c.Samples = 1
	} else {
		n := cur.Samples
		if n > tokenCalibrationWindow-1 {
			n = tokenCalibrationWindow - 1
		}
		c.Tokens = (cur.Tokens*n + c.Tokens) / (n + 1)
		c.Samples = cur.Samples + 1
	}
	c.UpdatedAt = time.Now()
	d.tokenCalibrations[c.Key] = c
	return c, atomicWriteJSON(d.dir(tokenCalibrationsFile), d.tokenCalibrations)
}

// TokenCalibrations returns a copy of every learned fact, keyed by Key.
func (d *DB) TokenCalibrations(ctx context.Context) map[string]TokenCalibration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]TokenCalibration, len(d.tokenCalibrations))
	for k, v := range d.tokenCalibrations {
		out[k] = v
	}
	return out
}

// loadTokenCalibrations reads the singleton document. An absent or unreadable
// file leaves the map empty: every fact is re-learned from the next turn (or
// re-counted), so a lost file costs one measurement, never correctness.
func (d *DB) loadTokenCalibrations() error {
	var m map[string]TokenCalibration
	if err := readJSONFile(d.dir(tokenCalibrationsFile), &m); err != nil {
		return nil
	}
	for k, v := range m {
		d.tokenCalibrations[k] = v
	}
	return nil
}
