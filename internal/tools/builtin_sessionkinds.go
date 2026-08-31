package tools

import (
	"fmt"
	"sort"
	"strings"
)

// Session.Kind values a session can carry. Every execution path funnels its
// transcript into a Session, and each entry point stamps its own kind:
//
//	chat     — user-initiated conversation (also the zero-value default)
//	spawned  — detached spawn (spawn tool / board automation)
//	worker   — coordinator-spawned worker (spawn with a coordinator session)
//	flow     — orchestration flow run transcript
//	task     — kanban task run transcript
//	schedule — cron/scheduler delivery transcript
//
// There is no dedicated peer-message kind: an agent's standing thread of messages
// from other agents is an ordinary "chat" session (TSK507, see
// internal/agent/agentmsg.go), so it archives and lists as a chat.
const (
	sessionKindChat     = "chat"
	sessionKindSpawned  = "spawned"
	sessionKindWorker   = "worker"
	sessionKindFlow     = "flow"
	sessionKindTask     = "task"
	sessionKindSchedule = "schedule"
)

// archivableSessionKinds is the full set archive_sessions can sweep — i.e. what
// `kinds:["*"]` expands to. It is the authoritative list used both to validate
// caller input and to render the tool's help text.
var archivableSessionKinds = []string{
	sessionKindChat,
	sessionKindSpawned,
	sessionKindWorker,
	sessionKindFlow,
	sessionKindTask,
	sessionKindSchedule,
}

// defaultArchiveKinds is what archive_sessions matches when the caller passes no
// `kinds` at all. It stays "chat"-only on purpose: before `kinds` existed the tool
// hardcoded that filter, so an existing "clean up old sessions" call must keep
// meaning exactly what it meant before and never start sweeping flow/schedule/
// inbox transcripts by surprise.
var defaultArchiveKinds = []string{sessionKindChat}

// resolveArchiveKinds turns the caller's `kinds` argument into the set of
// Session.Kind values to match.
//
//	nil/empty → defaultArchiveKinds (chat only — backward compatible)
//	["*"]     → every kind in archivableSessionKinds
//	["flow"]  → exactly that kind
//
// An unrecognised kind is a hard error rather than a silently-dropped filter: a
// typo like "spawn" (instead of "spawned") would otherwise match nothing and read
// as "there was nothing to archive", which is the opposite of the truth.
func resolveArchiveKinds(kinds []string) (map[string]struct{}, error) {
	cleaned := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			cleaned = append(cleaned, k)
		}
	}
	if len(cleaned) == 0 {
		cleaned = defaultArchiveKinds
	}

	set := make(map[string]struct{}, len(archivableSessionKinds))
	for _, k := range cleaned {
		if k == "*" {
			for _, all := range archivableSessionKinds {
				set[all] = struct{}{}
			}
			continue
		}
		if !isArchivableSessionKind(k) {
			return nil, fmt.Errorf("unknown session kind %q — valid kinds: %s (or \"*\" for all)",
				k, strings.Join(archivableSessionKinds, ", "))
		}
		set[k] = struct{}{}
	}
	return set, nil
}

// isArchivableSessionKind reports whether k is one of the known session kinds.
func isArchivableSessionKind(k string) bool {
	for _, known := range archivableSessionKinds {
		if k == known {
			return true
		}
	}
	return false
}

// sortedKinds renders a kind set as a stable, comma-separated string for the
// tool's output header, so the agent can see exactly which kinds were swept.
func sortedKinds(set map[string]struct{}) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
