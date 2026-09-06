package db

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
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
			d.markMutatedLocked()
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

	// 4) Remove orphan files left behind in the legacy uploads/ tree — anything no
	// longer referenced by a message attachment or an artifact. A file whose move
	// failed (still referenced via an uploads/ path) is kept, so nothing live is
	// lost. Then prune the emptied directories.
	d.cleanupOrphanUploads()
}

// cleanupOrphanUploads deletes files under workspace/uploads/ that are not
// referenced by any current message attachment (relPath) or artifact
// (SourcePath/ContentFile), then removes the now-empty directories. Safe: only
// unreferenced files are touched.
func (d *DB) cleanupOrphanUploads() {
	uploadsDir := filepath.Join(d.workspaceDir(), "uploads")
	if _, err := os.Stat(uploadsDir); err != nil {
		return
	}
	referenced := map[string]bool{}
	mark := func(rel string) {
		if rel != "" {
			referenced[filepath.ToSlash(rel)] = true
		}
	}
	for _, a := range d.artifacts {
		mark(a.SourcePath)
		mark(a.ContentFile)
	}
	for _, msgs := range d.messages {
		for _, m := range msgs {
			for _, at := range m.Attachments {
				mark(at.RelPath)
			}
		}
	}

	var dirs []string
	_ = filepath.WalkDir(uploadsDir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		rel, rerr := filepath.Rel(d.workspaceDir(), p)
		if rerr != nil {
			return nil
		}
		if !referenced[filepath.ToSlash(rel)] {
			_ = os.Remove(p)
		}
		return nil
	})
	// Remove directories deepest-first; os.Remove only succeeds when empty, so any
	// folder still holding a referenced file is preserved.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
}
