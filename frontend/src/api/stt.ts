// Server-side STT engine (optional whisper.cpp CLI + ffmpeg on the host). status
// lets the client decide between the server engine and the browser's Web Speech
// API; transcribe uploads recorded audio and returns recognized text — so a
// thin client / WebView2 build can dictate without browser speech recognition.
import { wsHeaders, errorFromResponse, req } from './client'

export interface ServerModel {
  id: string
  name: string
}

export interface SttStatus {
  available: boolean
  models: ServerModel[]
}

export const sttServerApi = {
  sttStatus: () => req<SttStatus>('/api/stt/status'),

  // POST recorded audio (any ffmpeg-decodable blob, e.g. webm/opus) → text.
  // Throws on a non-OK response so the caller can fall back to browser speech.
  transcribe: async (audio: Blob, lang: string, model: string): Promise<string> => {
    const qs = new URLSearchParams()
    if (lang) qs.set('lang', lang)
    if (model) qs.set('model', model)
    const headers = wsHeaders()
    // Send the raw audio; drop the JSON content-type wsHeaders sets by default.
    delete headers['Content-Type']
    const res = await fetch(`/api/stt?${qs.toString()}`, {
      method: 'POST',
      cache: 'no-store',
      headers,
      body: audio,
    })
    if (!res.ok) {
      throw new Error(await errorFromResponse(res))
    }
    const data = (await res.json()) as { text?: string }
    return data.text ?? ''
  },
}
