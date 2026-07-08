package tools

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
)

// verifyMutationLanded confirms that a file mutation the OS reported as
// successful actually LANDED: the on-disk bytes now equal what was written.
// os.WriteFile returning nil is not proof — antivirus interception, a
// concurrent writer, a dying disk or an overlay filesystem can leave stale or
// partial content behind while the write call "succeeds", and the agent then
// builds further steps on a change that never happened (the external-context-agent
// file-mutation-verifier lesson). A mismatch turns the tool call into an
// ERROR result, so the whole self-healing chain (guardrail counters,
// tool-error tag, lesson reflection) reacts to it like any real failure.
//
// Small files are compared byte-for-byte; large ones by SHA-256 of the full
// read-back (one extra read either way — mutations are rare relative to reads,
// so the cost is negligible).
func verifyMutationLanded(abs string, want []byte) error {
	got, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("file mutation did not land: %s reported written but cannot be read back (%v) — "+
			"the file may be locked or removed by another process; re-check the path and retry", abs, err)
	}
	if len(got) == len(want) {
		if len(want) <= 1<<20 { // ≤1MB: direct compare
			if bytes.Equal(got, want) {
				return nil
			}
		} else if sha256.Sum256(got) == sha256.Sum256(want) {
			return nil
		}
	}
	return fmt.Errorf("file mutation did not land: %s content on disk (%d bytes) does not match what was written (%d bytes) — "+
		"another process may have modified the file concurrently, or the filesystem intercepted the write; "+
		"Read the file to see its actual state before retrying", abs, len(got), len(want))
}
