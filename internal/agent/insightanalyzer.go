package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	steps        *insightStepRecorder
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
func analysisUserPrompt(req insight.AnalysisRequest, lang string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(req.Lens.Prompt))
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
		Messages:     []providers.Message{{Role: providers.RoleUser, Text: analysisUserPrompt(req, lang)}},
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
		sev := f.Severity
		if sev == "" {
			sev = "med"
		}
		out = append(out, insight.Finding{
			Signature:   sig,
			Title:       title,
			RootCause:   strings.TrimSpace(f.RootCause),
			ProposedFix: strings.TrimSpace(f.ProposedFix),
			FilePointer: strings.TrimSpace(f.FilePointer),
			Severity:    sev,
		})
	}
	return out
}

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
