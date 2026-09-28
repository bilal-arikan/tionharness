import { createContext, useContext } from 'react'
import { api } from '@/api'

type EditorApi = Pick<
  typeof api,
  'agentTools' | 'setAgentTools' | 'agentBuiltinPrompt' | 'agentContext'
>

export const AgentEditorContext = createContext<EditorApi | null>(null)
export function useAgentEditorApi(): EditorApi {
  return useContext(AgentEditorContext) ?? api
}
