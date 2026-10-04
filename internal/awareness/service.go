package awareness

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Service owns the per-session state the three moments need: the frozen brief
// (so the static prefix stays byte-stable between adopt points), the last pulse
// hash (so an unchanged pulse is not re-sent), the last turn composition (so the
// UI can show what the agent saw) and the digest (so an unchanged digest is not
// rewritten). One Service per workspace runtime.
type Service struct {
	store    Store
	root     string // workspace store root; the digest index lives under it
	settings func() Settings
	logger   *slog.Logger
	now      func() time.Time

	mu       sync.Mutex
	sessions map[string]*sessionState
	index    *digestIndex
	indexAt  time.Time
}

// sessionState is what the service remembers per session, mirrored to the
// awareness.json sidecar so the UI can show it after a restart.
type sessionState struct {
	Brief         *Composition `json:"brief,omitempty"`
	LastTurn      *Composition `json:"lastTurn,omitempty"`
	LastPulse     string       `json:"lastPulse,omitempty"`
	LastPulseHash string       `json:"lastPulseHash,omitempty"`
	PulseUrgent   bool         `json:"pulseUrgent,omitempty"`
	DigestHash    string       `json:"digestHash,omitempty"`
	Turns         int          `json:"turns"`
	UpdatedAt     int64        `json:"updatedAt"`
}

// Seen is the UI's view of what one session was told.
type Seen struct {
	SessionID string       `json:"sessionId"`
	Brief     *Composition `json:"brief,omitempty"`
	LastTurn  *Composition `json:"lastTurn,omitempty"`
	LastPulse string       `json:"lastPulse,omitempty"`
	Turns     int          `json:"turns"`
	Digest    *Digest      `json:"digest,omitempty"`
}

// New builds a service. root is the workspace store root (db.DB.Root()).
func New(store Store, root string, settings func() Settings, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if settings == nil {
		settings = DefaultSettings
	}
	return &Service{
		store: store, root: root, settings: settings, logger: logger, now: time.Now,
		sessions: map[string]*sessionState{},
	}
}

// Settings returns the normalized live settings.
func (s *Service) Settings() Settings { return s.settings().Normalized() }

// SetClock overrides the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now != nil {
		s.now = now
	}
}

// sidecar paths -------------------------------------------------------------------

const stateFile = "awareness.json"

func (s *Service) indexPath() string { return filepath.Join(s.root, "awareness", "digests.json") }

func (s *Service) sessionPath(sessionID, name string) string {
	dir, err := s.store.SessionDir(sessionID)
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

// state returns the in-memory state for a session, loading the sidecar once.
// Caller holds s.mu.
func (s *Service) state(sessionID string) *sessionState {
	if st, ok := s.sessions[sessionID]; ok {
		return st
	}
	st := &sessionState{}
	if p := s.sessionPath(sessionID, stateFile); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, st)
		}
	}
	s.sessions[sessionID] = st
	return st
}

// persistState mirrors the state to disk. Best-effort: a failed mirror only
// costs the UI after a restart, never the turn. Caller holds s.mu.
func (s *Service) persistState(sessionID string, st *sessionState) {
	st.UpdatedAt = s.now().Unix()
	p := s.sessionPath(sessionID, stateFile)
	if p == "" {
		return
	}
	if err := writeJSONAtomic(p, st); err != nil {
		s.logger.Debug("awareness state mirror failed", "session", sessionID, "error", err)
	}
}

// Brief ---------------------------------------------------------------------------

// Brief returns the session's briefing. It is composed on the first call (or
// when in.Fresh is set, or after InvalidateBrief) and then served from memory
// byte-for-byte, because the caller freezes it into the cached prompt prefix
// and compares live against frozen for drift: a brief that changed every turn
// would read as drift every turn.
//
// extra producers (api-side sections) run after the built-ins. A producer
// error skips that section and is logged.
func (s *Service) Brief(ctx context.Context, in Input, extra ...Producer) Composition {
	in.Settings = s.Settings()
	if !in.Settings.Enabled {
		return Composition{Moment: MomentBrief}
	}
	in.Digests = s
	if in.Now.IsZero() {
		in.Now = s.now()
	}
	s.mu.Lock()
	st := s.state(in.Session.ID)
	if st.Brief != nil && !in.Fresh {
		c := *st.Brief
		s.mu.Unlock()
		return c
	}
	s.mu.Unlock()

	producers := append(BriefProducers(), extra...)
	sections := s.runProducers(ctx, MomentBrief, in, producers)
	comp := Compose(MomentBrief, sections, in.Settings.BriefBudgetBytes)
	comp.At = in.Now.Unix()

	s.mu.Lock()
	st = s.state(in.Session.ID)
	st.Brief = &comp
	s.persistState(in.Session.ID, st)
	s.mu.Unlock()
	return comp
}

// Preview composes a brief for in without reading or writing any session
// state: the agent context preview wants "what would a fresh session see",
// and must not freeze a brief for a session that does not exist.
func (s *Service) Preview(ctx context.Context, in Input, extra ...Producer) Composition {
	in.Settings = s.Settings()
	if !in.Settings.Enabled {
		return Composition{Moment: MomentBrief}
	}
	in.Digests = s
	if in.Now.IsZero() {
		in.Now = s.now()
	}
	producers := append(BriefProducers(), extra...)
	comp := Compose(MomentBrief, s.runProducers(ctx, MomentBrief, in, producers), in.Settings.BriefBudgetBytes)
	comp.At = in.Now.Unix()
	return comp
}

// InvalidateBrief forgets the frozen brief so the next Brief call recomposes.
// Called where the prompt epoch adopts anyway (a compaction fold, a handoff).
func (s *Service) InvalidateBrief(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(sessionID)
	st.Brief = nil
	s.persistState(sessionID, st)
}

// runProducers runs each producer for the moment and collects its section.
func (s *Service) runProducers(ctx context.Context, moment Moment, in Input, producers []Producer) []Section {
	var out []Section
	for _, p := range producers {
		if !produces(p, moment) {
			continue
		}
		sec, err := p.Produce(ctx, in)
		if err != nil {
			s.logger.Warn("awareness producer failed", "moment", moment, "producer", p.Key(), "error", err)
			continue
		}
		if sec.Key == "" {
			sec.Key = p.Key()
		}
		if moment == MomentBrief && sec.Volatile {
			// A volatile section in the frozen brief would read as drift every
			// turn; refuse it rather than silently freezing stale data.
			s.logger.Warn("awareness: volatile section refused in brief", "producer", p.Key())
			continue
		}
		out = append(out, sec)
	}
	return out
}

func produces(p Producer, m Moment) bool {
	for _, x := range p.Moments() {
		if x == m {
			return true
		}
	}
	return false
}

// Turn ----------------------------------------------------------------------------

// Turn composes the volatile per-turn suffix: the caller's sections (clock,
// identity, recap …) in the order given, then the built-in turn producers, with
// the pulse de-duplicated against the previous turn. The whole thing is fitted
// to the turn budget and metered. The composition is remembered for the UI.
func (s *Service) Turn(ctx context.Context, in Input, lead []Section, trail ...Section) Composition {
	in.Settings = s.Settings()
	if in.Now.IsZero() {
		in.Now = s.now()
	}
	sections := append([]Section(nil), lead...)
	if in.Settings.Enabled {
		built := s.runProducers(ctx, MomentTurn, in, TurnProducers())
		s.mu.Lock()
		st := s.state(in.Session.ID)
		for _, sec := range built {
			if sec.Key == "pulse" {
				h := HashText(sec.Text)
				if h == st.LastPulseHash {
					continue // unchanged: say nothing
				}
				urgent := sec.Urgent
				if in.JudgeUrgent != nil {
					if u, ok := in.JudgeUrgent(ctx, sec.Text); ok {
						urgent = u
					}
				}
				if urgent {
					sec.Text = sec.Text + "\n[pulse: a change above needs attention before you continue the user's request]"
					sec.Priority = PriorityPinned
				}
				st.LastPulseHash = h
				st.LastPulse = sec.Text
				st.PulseUrgent = urgent
			}
			sections = append(sections, sec)
		}
		s.mu.Unlock()
	}
	sections = append(sections, trail...)
	comp := Compose(MomentTurn, sections, in.Settings.TurnBudgetBytes)
	comp.At = in.Now.Unix()
	s.mu.Lock()
	st := s.state(in.Session.ID)
	st.LastTurn = &comp
	st.Turns++
	s.persistState(in.Session.ID, st)
	s.mu.Unlock()
	return comp
}

// Digest --------------------------------------------------------------------------

// Digest computes the session's digest, persists it and updates the index when
// the facts changed. changed=false means the digest equals the last one and
// nothing was written.
func (s *Service) Digest(ctx context.Context, in Input) (Digest, bool, error) {
	in.Settings = s.Settings()
	if !in.Settings.Enabled {
		return Digest{}, false, nil
	}
	if in.Now.IsZero() {
		in.Now = s.now()
	}
	d, err := BuildDigest(ctx, in)
	if err != nil {
		return Digest{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(in.Session.ID)
	if st.DigestHash == d.Hash {
		return d, false, nil
	}
	if p := s.sessionPath(in.Session.ID, digestFile); p != "" {
		if err := writeJSONAtomic(p, d); err != nil {
			return d, false, err
		}
	}
	idx := s.loadIndexLocked()
	idx.upsert(d.indexEntry())
	if err := saveDigestIndex(s.indexPath(), *idx); err != nil {
		s.logger.Warn("awareness digest index write failed", "error", err)
	}
	st.DigestHash = d.Hash
	s.persistState(in.Session.ID, st)
	return d, true, nil
}

// loadIndexLocked reads the index once and refreshes it when the file changed
// under another process. Caller holds s.mu.
func (s *Service) loadIndexLocked() *digestIndex {
	path := s.indexPath()
	if s.index != nil {
		if st, err := os.Stat(path); err == nil && !st.ModTime().After(s.indexAt) {
			return s.index
		}
	}
	idx := loadDigestIndex(path)
	s.index = &idx
	if st, err := os.Stat(path); err == nil {
		s.indexAt = st.ModTime()
	} else {
		s.indexAt = s.now()
	}
	return s.index
}

// RecentDigests returns the newest digests (excluding one session), most
// recent first, within the recency window.
func (s *Service) RecentDigests(limit int, excludeSession string) []DigestIndexEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.loadIndexLocked()
	cutoff := s.now().Add(-sessionAge).Unix()
	var out []DigestIndexEntry
	for _, e := range idx.Entries {
		if e.SessionID == excludeSession || e.At < cutoff {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// AllDigests returns every indexed digest row, newest first (the Notes screen).
func (s *Service) AllDigests(limit int) []DigestIndexEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.loadIndexLocked()
	out := append([]DigestIndexEntry(nil), idx.Entries...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// LoadDigest reads a session's persisted digest.
func (s *Service) LoadDigest(sessionID string) (Digest, bool) {
	p := s.sessionPath(sessionID, digestFile)
	if p == "" {
		return Digest{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Digest{}, false
	}
	var d Digest
	if json.Unmarshal(b, &d) != nil {
		return Digest{}, false
	}
	return d, true
}

// Seen returns what one session was told, for the UI.
func (s *Service) Seen(sessionID string) Seen {
	s.mu.Lock()
	st := s.state(sessionID)
	out := Seen{SessionID: sessionID, Brief: st.Brief, LastTurn: st.LastTurn, LastPulse: st.LastPulse, Turns: st.Turns}
	s.mu.Unlock()
	if d, ok := s.LoadDigest(sessionID); ok {
		out.Digest = &d
	}
	return out
}

// Forget drops a session's state and index row (session deleted).
func (s *Service) Forget(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	idx := s.loadIndexLocked()
	idx.remove(sessionID)
	if err := saveDigestIndex(s.indexPath(), *idx); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Debug("awareness index prune failed", "error", err)
	}
}
