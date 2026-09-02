// useWorkspaceSignals bridges the workspace stream (ws:*, _Docs/77 R3) to the
// app's toasts, so a fact that happens while the user is looking at any screen
// still reaches them: an automation fired, a coordinator was halted by the
// stall guard, a trajectory started or ended, a flow run failed.
//
// It rides the lane store (connectLanes is ref-counted, one SSE connection is
// shared with the Rota screen and the chat header strip) and diffs its rings
// instead of opening a second subscription. Per-type mutes from Settings apply
// through the same NOTIFY_TYPES keys the global feed uses (automation,
// coordination, flow) plus the frontend-only `rota` type.
import { useEffect } from 'react'
import { toast } from '@/shared/components/toastStore'
import { isTypeEnabled } from '@/shared/lib/notifyPrefs'
import { connectLanes, getLanes, resetLanes, subscribeLanes } from '@/shared/lib/laneStore'
import { diffSignals } from './workspaceSignals'

export function useWorkspaceSignals(activeWorkspaceId: string | null): void {
  useEffect(() => {
    if (!activeWorkspaceId) return
    resetLanes()
    const release = connectLanes()
    let prev = getLanes()
    const unsub = subscribeLanes(() => {
      const next = getLanes()
      // A reset (stale) drops history; do not replay it as news.
      if (next.stale || next.revision < prev.revision) {
        prev = next
        return
      }
      for (const s of diffSignals(prev, next, isTypeEnabled)) {
        if (s.level === 'error') toast.error(s.text)
        else if (s.level === 'success') toast.success(s.text)
        else toast.info(s.text)
      }
      prev = next
    })
    return () => {
      unsub()
      release()
    }
  }, [activeWorkspaceId])
}
