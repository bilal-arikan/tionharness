package db

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// attachmentToArtifactKind maps a chat attachment's coarse kind to an artifact
// kind (mirrors the API-layer mapping, kept here for the migration).
func attachmentToArtifactKind(k string) string {
	switch k {
	case "image":
		return ArtifactImage
	case "video":
		return ArtifactVideo
	case "audio":
		return ArtifactAudio
	case "code":
		return ArtifactCode
	case "text":
		return ArtifactText
	default:
		return ArtifactFile
	}
}

// migrateUnifiedLayout moves pre-unification files into the per-session layout
// workspace/artifacts/<sessionDir>/ and makes existing chat attachments first-
// class artifacts. Idempotent — already-migrated paths (under artifacts/<sid>/)
// are skipped. Runs once at Open, single-threaded, after sessions + artifacts are
// loaded. Best-effort: a missing source file or failed move is left as-is.
func (d *DB) migrateUnifiedLayout() {
	wsDir := d.workspaceDir()

	// moveInto relocates a workspace-relative file to artifacts/<sessionDir>/<base>
	// and returns the new relative path. ok is false when nothing usable moved.
	moveInto := func(oldRel, sessionID, base string) (string, bool) {
		newRel := "artifacts/" + artifactSessionDir(sessionID) + "/" + base
		if oldRel == newRel {
			return newRel, true
		}
		oldAbs := filepath.Join(wsDir, filepath.FromSlash(oldRel))
		newAbs := filepath.Join(wsDir, filepath.FromSlash(newRel))
		if _, err := os.Stat(oldAbs); err != nil {
			// Source gone: if the destination already exists, treat as migrated.
			if _, derr := os.Stat(newAbs); derr == nil {
				return newRel, true
			}
			return oldRel, false
		}
		if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
			return oldRel, false
		}
		if err := os.Rename(oldAbs, newAbs); err != nil {
			return oldRel, false
		}
		return newRel, true
	}

	// 1+2) Artifacts: legacy flat content files (artifacts/<id><ext>) and media
	// under uploads/ → per-session folder.
	for id, a := range d.artifacts {
		changed := false
		if strings.HasPrefix(a.ContentFile, "artifacts/") && strings.Count(a.ContentFile, "/") == 1 {
			if nr, ok := moveInto(a.ContentFile, a.SessionID, path.Base(a.ContentFile)); ok && nr != a.ContentFile {
				a.ContentFile = nr
				changed = true
			}
		}
		if strings.HasPrefix(a.SourcePath, "uploads/") {
			if nr, ok := moveInto(a.SourcePath, a.SessionID, path.Base(a.SourcePath)); ok && nr != a.SourcePath {
				a.SourcePath = nr
				changed = true
			}
		}
		if changed {
			d.artifacts[id] = a
			_ = d.persistArtifactLocked(&a)
		}
	}

	// 3) Chat attachments in messages: move files under uploads/<sid>/ → per-session
	// folder, update the message relPath, and back each with a "chat" artifact.
	for sid, msgs := range d.messages {
		dirty := false
		for mi := range msgs {
			for ai := range msgs[mi].Attachments {
				att := &msgs[mi].Attachments[ai]
				if strings.HasPrefix(att.RelPath, "uploads/") {
					if nr, ok := moveInto(att.RelPath, sid, path.Base(att.RelPath)); ok && nr != att.RelPath {
						att.RelPath = nr
						dirty = true
					}
				}
				if att.RelPath != "" {
					_, _ = d.upsertAttachmentArtifactLocked(sid, msgs[mi].AgentID, att.RelPath, att.Name, attachmentToArtifactKind(att.Kind))
				}
			}
		}
		if dirty {
			if s, ok := d.sessions[sid]; ok {
				_ = d.writeSessionFileLocked(s)
			}
		}
	}
}
