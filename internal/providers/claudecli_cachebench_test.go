package providers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// benchStaticSystem is a deliberately non-trivial STATIC system prompt so there is
// a real cacheable prefix to measure. Cache reads only pay off when the warm prefix
// is large enough to matter; a one-line persona would make cache_read noise. Kept
// byte-stable across turns and across both modes so the ONLY variable is the process
// warmth model (respawn+resume vs persistent process).
const benchStaticSystem = `You are TionSwarm's benchmark assistant. Follow these standing rules on every turn:
- Be extremely terse. Never explain. Never add pleasantries.
- Treat any fact the user asks you to remember as durable session context.
- When asked to reply with a single word or token, output ONLY that token.
- Do not use tools unless explicitly required to answer.
- You operate inside an autonomous multi-agent runtime; other agents may read your output as raw data, so never wrap answers in prose.
- Preserve exact casing and punctuation of any codeword the user gives you.
- This paragraph is intentionally verbose to create a stable, cacheable system prefix so that turn-to-turn prompt-cache reuse can be measured accurately across the persistent-session and resume-based execution paths.`

// turnMetric is one measured turn: wall-clock plus the token accounting that reveals
// cache behaviour (cache_read cheap, cache_write premium, input fresh).
type turnMetric struct {
	turn      int
	wallMs    int64
	input     int
	cacheRead int
	cacheWr   int
	calls     int
	text      string
}

func (m turnMetric) row(mode string) string {
	return fmt.Sprintf("| %-16s | %4d | %8d | %10d | %11d | %10d | %5d |",
		mode, m.turn, m.wallMs, m.input, m.cacheRead, m.cacheWr, m.calls)
}

// TestLiveCacheCompareModes runs the SAME three logical turns through both claude-cli
// execution paths and prints a side-by-side cache + latency table:
//
//   - "respawn+resume": one-shot Complete per turn, threading the rotated SessionID
//     (a fresh claude subprocess each turn; server-side session restored via --resume).
//   - "persistent":     one warm CLISessionPool process fed over stdin stream-json
//     (no respawn; only the delta user message ships on warm turns).
//
// Both use the SAME static system prompt and the SAME volatile per-turn SystemDynamic
// (a changing clock), so the appended prefix is byte-identical and the only difference
// is the process-warmth model. Gated behind TIONSWARM_LIVE_CLI=1 (spends real tokens).
//
//	TIONSWARM_LIVE_CLI=1 go test ./internal/providers/ -run TestLiveCacheCompareModes -v -timeout 15m
func TestLiveCacheCompareModes(t *testing.T) {
	if os.Getenv("TIONSWARM_LIVE_CLI") != "1" {
		t.Skip("set TIONSWARM_LIVE_CLI=1 to run the live cache/latency comparison")
	}
	bin := "claude"
	if p := os.Getenv("TIONSWARM_CLAUDE_BIN"); p != "" {
		bin = p
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	// The three turns, expressed as their new-user-message text. The volatile clock is
	// supplied separately per turn so it lands in the message tail, not the cached prefix.
	turns := []string{
		"Remember this for later: my codeword is DELTA-7. Reply with just OK.",
		"What was the codeword I gave you? Reply with only the codeword.",
		"Reply with just the word DONE.",
	}

	respawn := runRespawnResume(ctx, t, bin, turns)
	// A brief gap so the persistent run starts from a comparable (warm-eligible) state
	// rather than racing the previous run's teardown. Well under the 5-min cache TTL.
	time.Sleep(2 * time.Second)
	persistent := runPersistent(ctx, t, bin, turns)

	// Emit the comparison table.
	var b strings.Builder
	b.WriteString("\n=== claude-cli cache/latency: respawn+resume vs persistent ===\n")
	b.WriteString("| mode             | turn | wall_ms  | input_tok  | cache_read  | cache_write | calls |\n")
	b.WriteString("|------------------|------|----------|------------|-------------|-------------|-------|\n")
	for _, m := range respawn {
		b.WriteString(m.row("respawn+resume") + "\n")
	}
	for _, m := range persistent {
		b.WriteString(m.row("persistent") + "\n")
	}
	b.WriteString(summarize("respawn+resume", respawn))
	b.WriteString(summarize("persistent", persistent))
	t.Log(b.String())

	// Sanity: both modes must retain context (turn 2 recalls the codeword). This keeps
	// the benchmark honest — a fast-but-amnesiac run is not a valid comparison.
	if len(respawn) >= 2 && !strings.Contains(strings.ToUpper(respawn[1].text), "DELTA-7") {
		t.Errorf("respawn+resume lost context on turn 2: %q", respawn[1].text)
	}
	if len(persistent) >= 2 && !strings.Contains(strings.ToUpper(persistent[1].text), "DELTA-7") {
		t.Errorf("persistent lost context on turn 2: %q", persistent[1].text)
	}
}

// runRespawnResume drives the turns via one-shot Complete, threading the rotated
// session id so each turn resumes the prior server-side session. Each turn sends ONLY
// its new user message (the server holds the history).
func runRespawnResume(ctx context.Context, t *testing.T, bin string, turns []string) []turnMetric {
	c := NewClaudeCLI(bin, "", "", "", "")
	var out []turnMetric
	var resumeID string
	for i, msg := range turns {
		start := time.Now()
		r, err := c.Complete(ctx, Request{
			PermissionMode:  "auto",
			System:          benchStaticSystem,
			SystemDynamic:   volatileClock(i),
			ResumeSessionID: resumeID,
			Messages:        []Message{{Role: RoleUser, Text: msg}},
		})
		wall := time.Since(start).Milliseconds()
		if err != nil {
			t.Fatalf("respawn+resume turn %d failed: %v", i+1, err)
		}
		resumeID = r.SessionID
		out = append(out, turnMetric{
			turn: i + 1, wallMs: wall, input: r.Usage.InputTokens,
			cacheRead: r.Usage.CacheReadTokens, cacheWr: r.Usage.CacheWriteTokens,
			calls: r.ProviderCalls, text: r.Text,
		})
	}
	return out
}

// runPersistent drives the turns through one warm CLISessionPool process. The full
// accumulated transcript is passed each turn; the pool ships only the delta on warm
// turns (the live process supplies the rest).
func runPersistent(ctx context.Context, t *testing.T, bin string, turns []string) []turnMetric {
	c := NewClaudeCLI(bin, "", "", "", "")
	pool := NewCLISessionPool()
	defer pool.Close()
	const key = "cachebench-persistent"

	var out []turnMetric
	var convo []Message
	for i, msg := range turns {
		convo = append(convo, Message{Role: RoleUser, Text: msg})
		start := time.Now()
		r, err := pool.Turn(ctx, key, c, Request{
			PermissionMode: "auto",
			System:         benchStaticSystem,
			SystemDynamic:  volatileClock(i),
			Messages:       convo,
		}, nil)
		wall := time.Since(start).Milliseconds()
		if err != nil {
			t.Fatalf("persistent turn %d failed: %v", i+1, err)
		}
		// Thread the assistant reply so the next turn's transcript is well-formed (the
		// pool only reads the last user message on warm turns, but a correct transcript
		// keeps a cold restart — e.g. a TTL evict mid-run — from losing history).
		convo = append(convo, Message{Role: RoleAssistant, Text: r.Text})
		out = append(out, turnMetric{
			turn: i + 1, wallMs: wall, input: r.Usage.InputTokens,
			cacheRead: r.Usage.CacheReadTokens, cacheWr: r.Usage.CacheWriteTokens,
			calls: r.ProviderCalls, text: r.Text,
		})
	}
	return out
}

// volatileClock returns a per-turn changing line. If this were folded into the cached
// system prefix it would bust the cache every turn; the whole point is that it rides
// the message tail instead.
func volatileClock(i int) string {
	return fmt.Sprintf("Current wall time: %s (turn %d — this line changes every turn).",
		time.Now().Format("2006-01-02 15:04:05.000"), i+1)
}

// summarize reports warm-turn (turns 2..N) aggregates — the steady-state a long
// session actually lives in, where the process-warmth difference shows up.
func summarize(mode string, ms []turnMetric) string {
	if len(ms) < 2 {
		return ""
	}
	var wall, read int64
	for _, m := range ms[1:] {
		wall += m.wallMs
		read += int64(m.cacheRead)
	}
	n := int64(len(ms) - 1)
	return fmt.Sprintf("[%s] warm-turn avg: wall=%dms cache_read=%d (over %d warm turns)\n",
		mode, wall/n, read/n, n)
}
