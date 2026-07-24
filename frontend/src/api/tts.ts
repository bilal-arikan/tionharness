// Server-side TTS engine (optional Piper CLI on the host). status lets the client
// decide between the server engine and the browser's speechSynthesis; synthesize
// returns WAV bytes so any client — including a phone — can play the audio the
// SERVER generated (no local voice needed).
import { wsHeaders, errorFromResponse, req } from './client'

export interface ServerVoice {
  id: string
  name: string
  lang: string
}

export interface TtsStatus {
  available: boolean
  voices: ServerVoice[]
}

export const ttsServerApi = {
  ttsStatus: () => req<TtsStatus>('/api/tts/status'),

  // POST text → WAV Blob. Throws on a non-OK response so the caller can fall back
  // to browser speech.
  synthesizeTts: async (text: string, voice: string): Promise<Blob> => {
    const res = await fetch('/api/tts', {
      method: 'POST',
      cache: 'no-store',
      headers: wsHeaders(),
      body: JSON.stringify({ text, voice }),
    })
    if (!res.ok) {
      throw new Error(await errorFromResponse(res))
    }
    return res.blob()
  },
}
