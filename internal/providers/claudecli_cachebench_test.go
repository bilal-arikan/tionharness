package providers

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// benchStaticBody is a deliberately non-trivial STATIC system prompt so there is a
// real cacheable prefix to measure. Cache reads only pay off when the warm prefix is
// large enough to matter; a one-line persona would make cache_read noise.
const benchStaticBody = `You are TionHarness's benchmark assistant. Follow these standing rules on every turn:
- Be extremely terse. Never explain. Never add pleasantries.
- Treat any fact the user asks you to remember as durable session context.
- When asked to reply with a single word or token, output ONLY that token.
- Do not use tools unless explicitly required to answer.
- You operate inside an autonomous multi-agent runtime; other agents may read your output as raw data, so never wrap answers in prose.
- Preserve exact casing and punctuation of any codeword the user gives you.
- This paragraph is intentionally verbose to create a stable, cacheable system prefix so that turn-to-turn prompt-cache reuse can be measured accurately across the persistent-session and resume-based execution paths.`

// benchStaticSystem returns the static system prompt seeded with a per-run, per-mode
// nonce line. The nonce is byte-stable WITHIN a run (so within-run cache reuse still
// works) but UNIQUE across runs and across the two modes — this is the hardening that
// removes the cross-run / cross-mode cache contamination the first measurement warned
// about (persistent turn-1 had read a prefix warmed by the respawn run). With distinct
// nonces each mode warms and measures ONLY its own cache.
func benchStaticSystem(nonce string) string {
	return "Benchmark run token: " + nonce + " (fixed for this run; ignore).\n\n" + benchStaticBody
}

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
// is the process-warmth model. Gated behind TIONHARNESS_LIVE_CLI=1 (spends real tokens).
//
//	TIONHARNESS_LIVE_CLI=1 go test ./internal/providers/ -run TestLiveCacheCompareModes -v -timeout 15m
func TestLiveCacheCompareModes(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_CLI") != "1" {
		t.Skip("set TIONHARNESS_LIVE_CLI=1 to run the live cache/latency comparison")
	}
	bin := "claude"
	if p := os.Getenv("TIONHARNESS_CLAUDE_BIN"); p != "" {
		bin = p
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Six turns (up from three) → four warm turns (2..6) per mode, a large enough
	// steady-state sample for a median to be meaningful. The recall turns (3, 5) also
	// assert context retention as the transcript grows.
	turns := []string{
		"Remember this for later: my codeword is DELTA-7. Reply with just OK.",
		"Reply with just the word READY.",
		"What was the codeword I gave you? Reply with only the codeword.",
		"Reply with just the word STEP.",
		"Repeat that codeword one more time. Reply with only the codeword.",
		"Reply with just the word DONE.",
	}

	// Per-mode unique nonce so neither mode reads a prefix the other (or a prior run)
	// warmed. UnixNano is fine in a Go test (the Date.now restriction is workflow-only).
	base := time.Now().UnixNano()
	respawn := runRespawnResume(ctx, t, bin, fmt.Sprintf("respawn-%d", base), turns)
	persistent := runPersistent(ctx, t, bin, fmt.Sprintf("persist-%d", base), turns)

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

	// Sanity: both modes must retain context on the recall turns (3 and 5, i.e. index
	// 2 and 4). A fast-but-amnesiac run is not a valid comparison.
	for _, idx := range []int{2, 4} {
		if len(respawn) > idx && !strings.Contains(strings.ToUpper(respawn[idx].text), "DELTA-7") {
			t.Errorf("respawn+resume lost context on turn %d: %q", idx+1, respawn[idx].text)
		}
		if len(persistent) > idx && !strings.Contains(strings.ToUpper(persistent[idx].text), "DELTA-7") {
			t.Errorf("persistent lost context on turn %d: %q", idx+1, persistent[idx].text)
		}
	}
}

// runRespawnResume drives the turns via one-shot Complete, threading the rotated
// session id so each turn resumes the prior server-side session. Each turn sends ONLY
// its new user message (the server holds the history).
func runRespawnResume(ctx context.Context, t *testing.T, bin, nonce string, turns []string) []turnMetric {
	c := NewClaudeCLI(bin, "", "", "", "")
	sys := benchStaticSystem(nonce)
	var out []turnMetric
	var resumeID string
	for i, msg := range turns {
		start := time.Now()
		r, err := c.Complete(ctx, Request{
			PermissionMode:  "auto",
			System:          sys,
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
func runPersistent(ctx context.Context, t *testing.T, bin, nonce string, turns []string) []turnMetric {
	c := NewClaudeCLI(bin, "", "", "", "")
	pool := NewCLISessionPool()
	defer pool.Close()
	sys := benchStaticSystem(nonce)
	const key = "cachebench-persistent"

	var out []turnMetric
	var convo []Message
	for i, msg := range turns {
		convo = append(convo, Message{Role: RoleUser, Text: msg})
		start := time.Now()
		r, err := pool.Turn(ctx, key, c, Request{
			PermissionMode: "auto",
			System:         sys,
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
// session actually lives in, where the process-warmth difference shows up. Median is
// reported alongside the mean because a single slow turn (TTL miss, server hiccup)
// skews a small-N mean; the median is the more honest steady-state figure.
func summarize(mode string, ms []turnMetric) string {
	if len(ms) < 2 {
		return ""
	}
	warm := ms[1:]
	walls := make([]int64, 0, len(warm))
	var wallSum, readSum int64
	for _, m := range warm {
		walls = append(walls, m.wallMs)
		wallSum += m.wallMs
		readSum += int64(m.cacheRead)
	}
	n := int64(len(warm))
	return fmt.Sprintf("[%s] warm turns=%d  wall: mean=%dms median=%dms  cache_read mean=%d\n",
		mode, n, wallSum/n, medianInt64(walls), readSum/n)
}

// medianInt64 returns the median of xs (sorted copy; average of the two middles for
// even length).
func medianInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}
