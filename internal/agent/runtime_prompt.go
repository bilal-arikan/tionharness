package agent

// System-prompt assembly: the agent persona (soul + identity) and the context
// blocks appended around it — environment, date/time, shell tools, working-dir
// confinement — plus the autonomous-turn variants. Kept together so the chat and
// autonomous paths can be read side by side and cannot silently drift apart.

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// BuildSystemPrompt composes the agent's persona from soul + identity. Exported
// as the SINGLE persona assembler: the chat/preview path (api package) uses it
// too, so the two paths can never drift apart.
func BuildSystemPrompt(a db.Agent) string {
	out := ""
	if a.Soul != "" {
		out = a.Soul
	}
	if a.Identity != "" {
		if out != "" {
			out += "\n\n"
		}
		out += a.Identity
	}
	return out
}

// EnvironmentContextBlock renders a one-line machine-environment marker (OS,
// arch, native shell) so the agent writes shell commands in the correct syntax
// instead of guessing — on Windows the shell tool is PowerShell, on Unix it is
// Bash (NewShellRunner picks the same identity). Mirrors the external agent project's
// <environment> marker, trimmed to the one field that actually changes agent
// behaviour (shell). Exported so both the chat path (api.composeTurnRequest) and
// the headless path (autonomousSystemPrompt) inject the identical line. It rides
// the volatile dynamic suffix, so it never disturbs the cached static prefix.
func EnvironmentContextBlock() string {
	// Prefer Bash when a POSIX shell backs it (always on Unix; on Windows only when a
	// bash.exe — Git Bash / WSL — is on PATH). PowerShell is advertised as the
	// fallback only for Windows-native tasks (cmdlets, registry, $env:). ShellToolNames
	// orders Bash first when present, so its first entry is the preferred shell and
	// can never claim "Bash" on a machine where bash.exe is missing.
	names := tools.ShellToolNames()
	shell := "Bash"
	if len(names) > 0 {
		shell = names[0]
	}
	hint := fmt.Sprintf("write shell commands in %s syntax for this machine.", shell)
	if shell == "Bash" && runtime.GOOS == "windows" {
		// Bash-first on Windows, but PowerShell stays available for native tasks.
		hint = "prefer Bash; use PowerShell only for Windows-native tasks (cmdlets, registry, `$env:`)."
	}
	return fmt.Sprintf("<environment os=%q arch=%q shell=%q /> — %s",
		runtime.GOOS, runtime.GOARCH, shell, hint)
}

// ShellToolsContextBlock states this session's shell-execution capability, and is
// the SINGLE source for it on both the chat (composeTurnRequest) and headless
// (autonomousDynamicSuffix) paths. It never returns empty:
//
//   - ENABLED — the shell gate is on (Tunables.ShellEnabled) AND a backing
//     interpreter is present (same resolvers as buildRegistry via
//     tools.ShellToolNames, so prompt ↔ catalog never drift) AND the agent's own
//     tool filter actually offers the tool: advertise only the shell tools that
//     survive all three, by their exact names.
//   - DISABLED — the gate is off OR no interpreter backs it OR the agent's
//     allow/denylist strips every shell tool: the Bash/PowerShell tools are NOT
//     registered for this agent, so say so explicitly and give the dead-tool rule.
//     Otherwise the model emits a bare `PowerShell`/`Bash` call, hits "No such
//     tool available … not enabled in this context", and — with nothing telling it
//     the tool is gone — repeats the identical call until the turn times out
//     (FND-9c9a52aa, FND-6095a777, FND-e9c79d9a, FND-495575b8).
//
// The per-agent filter is the SAME gate ToolCatalog applies (ToolAllowedFunc), so
// the block can never advertise a tool the agent's allowlist would strip: a
// read-only profile (allowlist without Bash/PowerShell) used to be told shell was
// ENABLED, called Bash, and got "No such tool available" on every attempt.
//
// It rides the VOLATILE dynamic suffix on both paths because the gate can toggle
// mid-session; the file tools (Read/Write/Edit/LS/Glob/Grep) stay the always-on
// core named in the static instructions.
func (r *Runtime) ShellToolsContextBlock(ctx context.Context, agent db.Agent, confined bool) string {
	names := r.availableShellToolNames(ctx, agent)
	if !r.tun.ShellEnabled() || len(names) == 0 {
		return "Shell execution is DISABLED for this session: there is NO Bash or PowerShell tool. " +
			"Do NOT call Bash or PowerShell — such a call fails with \"No such tool available\" / " +
			"\"not enabled in this context\". Any workspace guidance that assumes a terminal " +
			"(rtk wrappers, `go test`, `npm …`, shell one-liners) does NOT apply here. " +
			"General dead-tool rule: if ANY tool call returns \"No such tool available\" / " +
			"\"not enabled in this context\", treat that tool as absent — do NOT repeat the identical " +
			"call. Reach the goal with the file tools (Read / Glob / Grep / Edit / Write), or report " +
			"that the step needs a shell that is not available in this context."
	}
	// Name the tools as THIS agent will actually call them. On a CLI turn the bare
	// name is not merely unqualified — claude-cli's OWN native Bash is suppressed
	// once TionHarness's shell is bridged (climcp.go), so a bare `Bash` call dies
	// with "No such tool available: Bash. Bash is disabled for this session" and
	// the model has to guess the namespace to recover.
	names = shellToolNamesFor(agent.Provider, names)
	noun, verb := "tool", "runs"
	if len(names) > 1 {
		noun, verb = "tools", "run"
	}
	scope := "not confined to the working directory (absolute paths and `..` allowed)"
	if confined {
		scope = "confined to the working directory (write relative paths; an in-root absolute path is allowed, but escapes are rejected)"
	}
	return "Shell execution is ENABLED for this session: the " + strings.Join(names, " / ") + " " + noun +
		" " + verb + " host commands — " + scope + ", " +
		"with the permission mode as the safety layer. Call " + strings.Join(names, " / ") + " by that exact name."
}

// shellToolNamesFor renders shell tool names the way the given provider exposes
// them: bare for a native (API) agent, Interaction-MCP-namespaced for a CLI one.
// Same reasoning as skillToolNameFor, and the same forms climcp_matcher.go
// matches hooks against — the allow/denylist gating in availableShellToolNames
// stays on the BARE names, because that is what the tool filter is keyed by.
func shellToolNamesFor(provider string, names []string) []string {
	if !isCLIProviderKind(provider) {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, interactionToolPrefix+n)
	}
	return out
}

// availableShellToolNames narrows the host's backing shell interpreters
// (tools.ShellToolNames) to the ones THIS agent may actually call, by running each
// name through the agent's effective tool filter — the same predicate ToolCatalog
// uses. Host support and the agent's allow/denylist are independent gates: a
// machine can have bash.exe while the agent's profile allowlist omits "Bash", and
// only the intersection is real. Returns nil when the agent can call neither.
func (r *Runtime) availableShellToolNames(ctx context.Context, agent db.Agent) []string {
	allowed := r.ToolAllowedFunc(ctx, agent)
	var names []string
	for _, n := range tools.ShellToolNames() {
		if allowed(n) {
			names = append(names, n)
		}
	}
	return names
}

// systemPrompt builds an agent's static system prefix: its soul+identity persona
// followed by this workspace's instructions (when set). Both are stable, so they
// belong in the cached static prefix rather than the volatile dynamic suffix.
func (r *Runtime) systemPrompt(a db.Agent) string {
	out := BuildSystemPrompt(a)
	if p := r.instructions.Load(); p != nil {
		if ins := strings.TrimSpace(*p); ins != "" {
			if out != "" {
				out += "\n\n"
			}
			out += "# Workspace Instructions\n" + ins
		}
	}
	// Terse mode rides the same static prefix, AFTER the workspace instructions so
	// a workspace rule can still be phrased to override the reply style.
	if tb := r.TerseModeBlock(); tb != "" {
		if out != "" {
			out += "\n\n"
		}
		out += tb
	}
	return out
}

// autonomousSystemPrompt is systemPrompt plus the agent's Available Skills block,
// for headless runs (scheduler/spawn/flow). Chat turns add the catalog
// in composeTurnRequest; the autonomous entry points (which build their own
// request) had no catalog, so a scheduled agent never learned its skills. Adding
// it here — together with the autonomous Interaction use_skill bridge — gives
// headless runs the same skill access chat agents have.
func (r *Runtime) autonomousSystemPrompt(ctx context.Context, a db.Agent) string {
	cwd := r.sessionCwd(ctx)
	// PURE builder: the prompt epoch calls it every turn for drift detection but
	// only ships its output at adopt points, so side effects must stay out here.
	build := func() string {
		out := r.systemPrompt(a)
		if sb := r.SkillsCatalogBlockForAgent(a); sb != "" {
			out = strings.TrimSpace(out + "\n\n" + sb)
		}
		// Advertise the agent's LAZY tools (self-management + MCP) as a load-on-demand
		// catalog. Without it a headless turn calls a deferred tool whose schema was
		// never loaded and fails with InputValidationError. Ordered right after the
		// skills block to match the chat path (api.composeTurnRequest), so both paths
		// produce the same cached static prefix.
		if tb := r.LazyToolsCatalogBlock(ctx, a); tb != "" {
			out = strings.TrimSpace(out + "\n\n" + tb)
		}
		// Advertise optional external-tool capabilities (e.g. codebase-memory) present
		// in this workspace so a headless turn reaches for them too, WITH the session's
		// cwd-derived project id (ctx carries the session id on scheduler/spawn/flow
		// paths). Presence is stable, so it rides the cached static prefix. Shares ONE
		// source with the chat path (api.composeTurnRequest).
		if cb := r.CapabilityContext(ctx, a, cwd); cb != "" {
			out = strings.TrimSpace(out + "\n\n" + cb)
		}
		// Boot/verification sequence (Anthropic long-running-agent harness discipline):
		// a headless turn starts with a fresh context, so nudge it through the fixed
		// orient → recall → select-one → verify-baseline → work → close-the-loop routine
		// before acting. We inject only a pointer to keep the cached prefix small; the
		// full recipe lives in the tionharness-autonomous-ops skill.
		if r.tun.AutonomousBootSeq() {
			out = strings.TrimSpace(out + "\n\n" + autonomousBootReminder)
		}
		// Machine-environment marker (OS/arch/shell) so a headless turn writes shell
		// commands in the right syntax. Its bytes never change within a process, so
		// it is safe inside the cached static prefix. The VOLATILE pieces (turn-start
		// clock, lessons) deliberately live in autonomousDynamicSuffix — putting
		// them here would change the prefix bytes every turn and defeat prompt
		// caching for every headless run.
		return strings.TrimSpace(out + "\n\n" + EnvironmentContextBlock())
	}
	// Best-effort: ensure the session's repo is indexed in this workspace's isolated
	// store (guarded once per cwd per process; no-op without a cwd or an enabled
	// codebase-memory server). Outside the builder — it must run on frozen turns too.
	r.EnsureCodebaseIndexed(ctx, cwd)
	// Serve through the prompt epoch (frozen snapshot) keyed to this session, so a
	// headless run's prefix is as drift-proof as a chat turn's. Headless sessions
	// are single-agent (multiAgent=false); drift is surfaced by the dynamic suffix
	// via PromptEpochStale (autonomousDynamicSuffix).
	sys, _ := r.EpochStaticSystem(ctx, SessionIDFrom(ctx), a, false, false, cwd, build)
	return sys
}

// DateTimeContextBlock renders the turn-start clock line. Exported so the chat
// path (api.composeTurnRequest) and the headless dynamic suffix share ONE
// wording. It is volatile by nature, so it must ride SystemDynamic — never the
// cached static prefix.
func DateTimeContextBlock() string {
	return "Current date and time (captured at the start of this turn; seconds-precise, does not tick mid-turn): " +
		time.Now().Format("Monday, 2006-01-02 15:04:05 (-07:00)")
}

// autonomousDynamicSuffix builds the VOLATILE system suffix for headless turns
// (scheduler/spawn/flow/subagent): the turn-start clock plus the newest failure
// lessons. Chat turns assemble the same pieces in composeTurnRequest; keeping
// them out of autonomousSystemPrompt keeps the static prefix byte-stable across
// turns so cache-capable providers reuse it.
//
// agent is the turn's own agent: the shell-capability block below is per-agent
// (its allow/denylist decides whether Bash/PowerShell are offered at all), so it
// cannot be derived from the session id alone.
func (r *Runtime) autonomousDynamicSuffix(ctx context.Context, agent db.Agent) string {
	out := DateTimeContextBlock()
	// Failure lessons (hata→ders döngüsü): the newest distilled lessons ride
	// every headless turn so a fresh context does not repeat known failures.
	// The turn's own agent (resolved via the stamped session) ranks first.
	agentID, isCoordinator := "", false
	sid := SessionIDFrom(ctx)
	if sid != "" {
		if sess, err := r.db.GetSession(ctx, sid); err == nil {
			agentID = sess.AgentID
			isCoordinator = sess.IsCoordinator()
		}
	}
	if lb := r.LessonsContextBlock(ctx, agentID); lb != "" {
		out += "\n\n" + lb
	}
	// Working-directory context: the chat path injects workdirContextBlock, but a
	// headless turn had none — so an autonomous worker learned its root and the
	// relative-path rule only by trial (a rejected in-root absolute, a mis-rooted
	// guess). State it up front, and describe the confinement when the autonomous
	// brake is on. Volatile (the brake is a toggamble tunable) → dynamic suffix.
	confined := r.tun != nil && r.tun.AutonomousConfine()
	if wb := r.workdirConfineBlock(ctx, confined); wb != "" {
		out += "\n\n" + wb
	}
	// Shell-execution capability, single-sourced with the chat path: advertises
	// the registered Bash/PowerShell tools when the gate is on + a shell backs it,
	// else states shell is disabled and gives the dead-tool rule. Volatile →
	// dynamic suffix, since the gate can toggle mid-session. The confined flag keeps
	// its "confined/not confined" clause consistent with the block above.
	if sh := r.ShellToolsContextBlock(ctx, agent, confined); sh != "" {
		out += "\n\n" + sh
	}
	// Prompt-epoch drift notice (mirrors the chat path): the frozen snapshot is
	// holding back a live change — a compact diff on the volatile side. The suffix
	// runs after autonomousSystemPrompt in request composition, so the diff (set by
	// EpochStaticSystem) is fresh for this turn.
	if sid != "" {
		if note := r.PromptEpochContextNote(sid, agentID); note != "" {
			out += "\n\n" + note
		}
	}
	// Coordinator turns get an authoritative live SITUATION block — fleet state,
	// the agent roster with write capability, and the board — so the model can
	// never believe a finished worker is still running (the coalesced-notification
	// stall) and does not spend two or three tool calls a turn re-reading state
	// that is already here. Coordinator-only, reusing the session loaded above —
	// which includes a mid-level node, whose block also reports its own subtree.
	if sid != "" && isCoordinator {
		if wb := r.coordinatorSituationBlock(ctx, sid); wb != "" {
			out += "\n\n" + wb
		}
	}
	return out
}

// workdirConfineBlock renders the headless turn's working-directory context: its
// root and, when confined is true, the rule that the fs/shell tools cannot leave
// it (write relative paths; an in-root absolute is accepted, escapes rejected).
// Mirrors the chat path's workdirContextBlock, which a headless turn never got —
// so an autonomous worker no longer discovers the boundary by a rejected path.
// The root is taken from the same source the sandbox roots at: the turn's resolved
// working dir when ctx carries it, else the session's effective working dir.
func (r *Runtime) workdirConfineBlock(ctx context.Context, confined bool) string {
	dir := ""
	if rw, ok := resolvedWorkDirFromCtx(ctx); ok && strings.TrimSpace(rw.dir) != "" {
		dir = rw.dir
	} else {
		dir = r.effectiveWorkDir(ctx)
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Working directory\n")
	if confined {
		b.WriteString("Your file and shell tools are CONFINED to this directory. Write paths relative to it; " +
			"an absolute path inside it is accepted, but any path that escapes it (an outside absolute path or a " +
			"`..` traversal) is rejected. Orient with Glob/Grep before guessing a path.\n\n")
	} else {
		b.WriteString("Your file and shell tools operate from this directory. Relative paths resolve here; " +
			"you may also use absolute paths.\n\n")
	}
	b.WriteString("- Path: `" + dir + "`\n")
	return strings.TrimSpace(b.String())
}

// autonomousBootReminder frames every headless turn (schedule/spawn/flow/
// subagent). Deliberately stated as GOALS AND BOUNDARIES rather than a numbered
// step recipe: current-generation models (Fable 5 class) follow intent well and
// over-prescriptive scaffolding measurably reduces their output quality. Three
// concerns, per Anthropic's long-running-agent guidance: (1) autonomy — no user
// is watching, act instead of asking or ending on a plan; (2) grounded progress
// — claims must be backed by a tool result from this session; (3) durable
// closure — record what changed so the next fresh context can pick it up. The
// detailed recipe stays in the tionharness-autonomous-ops skill.
const autonomousBootReminder = "# Autonomous operation\n" +
	"This is a headless turn with a fresh context; no user is watching and none can answer questions, " +
	"so do not ask permission and do not end the turn with a plan or a promise — for reversible actions " +
	"that follow from the task, act. Orient yourself before changing anything (working directory, git state, " +
	"the persisted progress file, open tasks) and verify the baseline is green before building on it; " +
	"fix a broken baseline first. Work on ONE piece of work per turn, done properly, rather than several half-done. " +
	"Before reporting progress, check each claim against a tool result from this session — report only what you can " +
	"point to evidence for, and say explicitly when something is not yet verified. Close the loop when finished: " +
	"commit/record what changed and append a progress note (never overwrite a prior note). " +
	"Playbook when needed: use_skill \"tionharness-autonomous-ops\"."
