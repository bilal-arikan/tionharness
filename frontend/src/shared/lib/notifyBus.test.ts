import { describe, expect, it, vi, beforeEach } from 'vitest'

// notifyBus.playCue/emitToast route sound + OS toast through one funnel — these
// tests pin the 'permission' cue override so a future refactor can't silently
// route tool-approval prompts back onto the plain 'ask' chime.
vi.mock('./sounds', () => ({
  playTurnDone: vi.fn(),
  playAskPrompt: vi.fn(),
  playPermissionPrompt: vi.fn(),
}))

vi.mock('./clientPrefs', () => ({
  notify: vi.fn(),
}))

vi.mock('./notifyPrefs', () => ({
  isTypeEnabled: vi.fn(() => true),
}))

import { playTurnDone, playAskPrompt, playPermissionPrompt } from './sounds'
import { notify } from './clientPrefs'
import { playCue, emitToast } from './notifyBus'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('playCue', () => {
  it('plays the reply-ready chime for a completed chat turn', () => {
    playCue('chat')
    expect(playTurnDone).toHaveBeenCalledOnce()
    expect(playAskPrompt).not.toHaveBeenCalled()
    expect(playPermissionPrompt).not.toHaveBeenCalled()
  })

  it('plays the plain ask cue for a prompt with no override', () => {
    playCue('prompt')
    expect(playAskPrompt).toHaveBeenCalledOnce()
    expect(playPermissionPrompt).not.toHaveBeenCalled()
  })

  it('plays the permission cue when overridden, instead of the type default', () => {
    playCue('prompt', 'permission')
    expect(playPermissionPrompt).toHaveBeenCalledOnce()
    expect(playAskPrompt).not.toHaveBeenCalled()
  })

  it('stays silent for a type with no cue', () => {
    playCue('board')
    expect(playTurnDone).not.toHaveBeenCalled()
    expect(playAskPrompt).not.toHaveBeenCalled()
    expect(playPermissionPrompt).not.toHaveBeenCalled()
  })
})

describe('emitToast', () => {
  it('routes a permission-cue request through playPermissionPrompt and still raises the OS toast', () => {
    emitToast({
      type: 'prompt',
      enabled: true,
      title: 'İzin bekleniyor',
      body: 'Bash: rm -rf',
      cue: 'permission',
    })
    expect(playPermissionPrompt).toHaveBeenCalledOnce()
    expect(playAskPrompt).not.toHaveBeenCalled()
    expect(notify).toHaveBeenCalledOnce()
  })

  it('falls back to the ask cue for a plain question (no override)', () => {
    emitToast({
      type: 'prompt',
      enabled: true,
      title: 'Ajan bir soru sordu',
      body: '',
    })
    expect(playAskPrompt).toHaveBeenCalledOnce()
    expect(playPermissionPrompt).not.toHaveBeenCalled()
  })
})
