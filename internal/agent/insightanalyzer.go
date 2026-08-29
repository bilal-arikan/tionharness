package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// insightAnalyzer implements insight.Analyzer using a cheap model through the
// runtime's guardedComplete funnel (usage-metered, workspace-pinned) — the same
// path the lesson reflector uses. It asks for a JSON array of findings via
// structured output; models without structured-output support return free text,
// so the reply is parsed with a fallback (unparseable → no findings, logged).
type insightAnalyzer struct {
	rt     *Runtime
	agent  db.Agent
	system string
	// steps, when set, receives the model's verbatim reply so the scan's live
	// transcript can show it. It is fed HERE rather than through the
	// insight.Analyzer interface: that interface returns parsed findings only, and
	// widening it would force every fake analyzer to carry raw-response plumbing.
	steps *insightStepRecorder
	// findings is the run's finding store, read-only here: its signatures are
	// offered back to the model so it reuses them instead of minting a new slug
	// for a problem already on record. Optional (nil in tests without a store).
	findings     *insight.FindingStore
	mu           sync.Mutex
	permanentErr error
}

// The analyzer's fallback system prompt lives in the central prompt registry
// under "insight-analyzer". The configured insight system agent may override it.

// insightFindingsSchema constrains the reply to a findings array (structured
// output on capable models; ignored elsewhere).
var insightFindingsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "title":       { "type": "string" },
          "rootCause":   { "type": "string" },
          "proposedFix": { "type": "string" },
          "filePointer": { "type": "string" },
          "severity":    { "type": "string", "enum": ["low", "med", "high"] },
          "signature":   { "type": "string" }
        },
        "required": ["title"]
      }
    }
  },
  "required": ["findings"]
}`)

// analysisUserPrompt assembles the analyzer's user message: the lens prompt, the
// session evidence, the strict-JSON instruction, and — when lang is non-empty — a
// directive to write the user-facing prose fields in that language (code
// identifiers, paths and the signature stay verbatim so dedup/file-pointers hold).
// knownSigs, when non-empty, lists signatures already on record for this lens's
// topic so the model reuses one instead of minting a fresh slug for a problem
// that is already tracked (the store's similarity dedupe is the safety net, not
// the first line of defence).
func analysisUserPrompt(req insight.AnalysisRequest, lang string, knownSigs []string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(req.Lens.Prompt))
	if len(knownSigs) > 0 {
		b.WriteString("\n\n--- SIGNATURES ALREADY ON RECORD ---\n")
		for _, s := range knownSigs {
			b.WriteString("- " + s + "\n")
		}
		b.WriteString("If a finding is the same problem as one of these, reuse that EXACT signature so it " +
			"updates the existing entry. Only invent a new signature for a genuinely new problem.")
	}
	b.WriteString("\n\n--- SESSION EVIDENCE ---\n")
	b.WriteString(req.Transcript)
	b.WriteString("\n\nReturn ONLY JSON of the form {\"findings\":[{\"title\":...,\"rootCause\":...,\"proposedFix\":...,\"filePointer\":...,\"severity\":\"low|med|high\",\"signature\":...}]}. " +
		"Use a STABLE signature per problem shape (tool/error shape, not volatile ids). Empty array if nothing qualifies.")
	if lang != "" {
		b.WriteString(" Write the title, rootCause and proposedFix in " + lang +
			"; keep the signature, code identifiers and file paths verbatim (do not translate them).")
	}
	return b.String()
}

func (a *insightAnalyzer) Analyze(ctx context.Context, req insight.AnalysisRequest) ([]insight.Finding, error) {
	a.mu.Lock()
	terminalErr := a.permanentErr
	a.mu.Unlock()
	if terminalErr != nil {
		return nil, terminalErr
	}
	lang := ""
	if a.rt != nil && a.rt.tun != nil {
		lang = a.rt.tun.Language()
	}
	resp, err := a.rt.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindReflect), "insight-analyzer", a.system), a.agent, providers.Request{
		Model:        a.agent.Model,
		System:       a.system,
		MaxTokens:    1500,
		OutputSchema: insightFindingsSchema,
		Messages:     []providers.Message{{Role: providers.RoleUser, Text: analysisUserPrompt(req, lang, a.knownSigsFor(req.Lens))}},
	}, false)
	if err != nil {
		if errors.Is(err, providers.ErrPermanentProviderFailure) {
			a.mu.Lock()
			if a.permanentErr == nil {
				a.permanentErr = err
			}
			err = a.permanentErr
			a.mu.Unlock()
		}
		return nil, err
	}
	a.steps.captureRaw(req.Lens.ID, req.SessionID, resp.Text)
	return a.parse(resp.Text, req.Lens), nil
}

// knownSigCount bounds how many stored signatures ride the analysis prompt:
// enough to cover the live topics, small enough not to crowd the evidence.
// Newest first, since a recurring failure is a recent one.
const knownSigCount = 30

// knownSigsFor returns the signatures to offer the model for reuse. EVERY lens
// gets them: without the hint the analyzer mints a fresh slug each run and the
// same problem fragments into a new card per scan. The lens's own findings come
// first, then the rest of its channel (one root cause is often already filed by
// a sibling lens); the lessons-mining lens additionally gets the lesson
// signatures its findings were promoted into.
func (a *insightAnalyzer) knownSigsFor(lens insight.Lens) []string {
	sigs := make([]string, 0, knownSigCount)
	seen := make(map[string]bool, knownSigCount)
	add := func(sig string) {
		sig = strings.TrimSpace(sig)
		if sig == "" || seen[sig] || len(sigs) >= knownSigCount {
			return
		}
		seen[sig] = true
		sigs = append(sigs, sig)
	}
	if lens.ID == lessonsMiningLensID && a.rt != nil && a.rt.db != nil {
		lessons, err := a.rt.db.ListLessons(knownSigCount)
		if err != nil {
			a.rt.logger.Warn("insight analyzer: known lesson signatures unavailable", "error", err)
		}
		for _, l := range lessons {
			if strings.HasPrefix(l.Signature, lessonInsightSignaturePrefix) {
				add(l.Signature)
			}
		}
	}
	if a.findings != nil {
		for _, f := range a.findings.List(lens.ID, "") {
			add(f.Signature)
		}
		if lens.Channel != "" {
			for _, f := range a.findings.List("", lens.Channel) {
				add(f.Signature)
			}
		}
	}
	return sigs
}

func (a *insightAnalyzer) permanentError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.permanentErr
}

// parse maps the model reply into findings, filling channel-independent fields;
// the scanner stamps LensID/Channel/timestamps. An unparseable reply yields no
// findings (logged, not an error — the scanner would otherwise retry forever).
func (a *insightAnalyzer) parse(text string, lens insight.Lens) []insight.Finding {
	raw := extractJSONObject(text)
	if raw == "" {
		a.rt.logger.Warn("insight analyzer: no JSON in reply", "lens", lens.ID)
		return nil
	}
	var parsed struct {
		Findings []struct {
			Title       string `json:"title"`
			RootCause   string `json:"rootCause"`
			ProposedFix string `json:"proposedFix"`
			FilePointer string `json:"filePointer"`
			Severity    string `json:"severity"`
			Signature   string `json:"signature"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		a.rt.logger.Warn("insight analyzer: JSON parse failed", "lens", lens.ID, "error", err)
		return nil
	}
	out := make([]insight.Finding, 0, len(parsed.Findings))
	for _, f := range parsed.Findings {
		title := strings.TrimSpace(f.Title)
		if title == "" {
			continue
		}
		sig := strings.TrimSpace(f.Signature)
		if sig == "" {
			sig = fallbackSignature(lens.ID, title)
		}
		out = append(out, insight.Finding{
			Signature:   sig,
			Title:       title,
			RootCause:   strings.TrimSpace(f.RootCause),
			ProposedFix: strings.TrimSpace(f.ProposedFix),
			FilePointer: sanitizeFilePointer(f.FilePointer),
			Severity:    normalizeSeverity(f.Severity),
		})
	}
	return out
}

// normalizeSeverity maps whatever the model wrote into the schema's enum
// (low|med|high). Models routinely answer "medium", "critical" or "P2"; storing
// those verbatim breaks PriorityScore's severity weighting, which only knows the
// three canonical values. Anything unrecognised falls back to "med".
func normalizeSeverity(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "low", "minor", "trivial", "info", "nit", "p3":
		return "low"
	case "high", "critical", "crit", "blocker", "severe", "urgent", "major", "p0", "p1":
		return "high"
	default:
		// "med", "medium", "moderate", "normal", "" and anything unknown.
		return "med"
	}
}

// sanitizeFilePointer drops a pointer that names a SESSION rather than a file.
// The analyzer is asked for a repo-relative path but sometimes cites its
// evidence instead ("SESSION SES2047", "session SES12/msg3"); storing that makes
// the UI render a dead file link and makes insight.CheckFilePointer report the
// finding as an unverified path when there is no path at all.
func sanitizeFilePointer(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	first := strings.ToLower(strings.Fields(p)[0])
	first = strings.Trim(first, ":,")
	if first == "session" || first == "sessions" || sessionIDRef.MatchString(first) {
		return ""
	}
	return p
}

// sessionIDRef matches a bare session identifier ("ses2047", "ses2047/msg3").
var sessionIDRef = regexp.MustCompile(`^ses\d+(\b|/|$)`)

// fallbackSignature builds a stable dedupe key from the lens + normalized title
// when the model omitted one.
func fallbackSignature(lensID, title string) string {
	norm := strings.Join(strings.Fields(strings.ToLower(title)), " ")
	sum := sha256.Sum256([]byte(norm))
	return lensID + ":" + hex.EncodeToString(sum[:6])
}

// extractJSONObject returns the outermost {...} span of s (models sometimes wrap
// JSON in prose or code fences). Empty when no balanced object is present.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// inside string literal, ignore braces
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
