import { Toggle, Segmented } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function CliOptions({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle
        label="Forward hooks to Claude CLI"
        hint="Run workspace hooks through the CLI. Commands must be compatible with the CLI shell."
        checked={draft.enableCliHooks}
        onChange={(v) => set('enableCliHooks', v)}
      />
      <Segmented
        label="Claude CLI session mode"
        value={draft.claudePersistentSession ? 'persistent' : draft.claudeResume ? 'resume' : 'off'}
        onChange={(mode) => {
          // Two mutually-exclusive booleans drive the runtime (toolloop.go picks
          // persistent when on; chat_resume.go gates --resume only when persistent is
          // off). Map each segment to a deterministic pair so exactly one path is live.
          set('claudePersistentSession', mode === 'persistent')
          set('claudeResume', mode === 'persistent' || mode === 'resume')
        }}
        options={[
          {
            value: 'persistent',
            label: 'Persistent process',
            hint: 'Keep one CLI process alive per session and send only new messages.',
          },
          {
            value: 'resume',
            label: 'Resume',
            hint: 'Start a process per turn and resume the previous CLI session. Available for single-agent conversations.',
          },
          {
            value: 'off',
            label: 'Off',
            hint: 'Start a new process with the full transcript each turn. Useful for diagnostics; increases input usage.',
          },
        ]}
      />
      <Toggle
        label="Pass the system prompt through a file"
        hint="Use a temporary file to avoid command-line length limits with large prompts."
        checked={draft.claudeSysPromptFile}
        onChange={(v) => set('claudeSysPromptFile', v)}
      />
      <Toggle
        label="Limit Claude CLI built-in tools"
        hint="Expose only the built-in tools needed by the bridge to reduce prompt overhead."
        checked={draft.claudeCliToolAllowlist}
        onChange={(v) => set('claudeCliToolAllowlist', v)}
      />
      <Toggle
        label="Claude CLI research agents"
        hint="Allow native Explore and Plan agents. Delegation to TionHarness agents stays separate."
        checked={draft.claudeCliNativeSubagents}
        onChange={(v) => set('claudeCliNativeSubagents', v)}
      />
      <Toggle
        label="Use Anthropic API for auxiliary calls"
        hint="Route supported title, summary and other tool-free system calls through an available Anthropic API instance. These calls use API billing instead of the CLI subscription."
        checked={draft.auxNativeRouting}
        onChange={(v) => set('auxNativeRouting', v)}
      />
    </>
  )
}
