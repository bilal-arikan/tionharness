import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/api'
import { sttModel } from '@/shared/lib/stt'

export interface ServerStt {
  // recording audio from the mic right now.
  listening: boolean
  // recording stopped; audio is being transcribed on the server.
  transcribing: boolean
  error: string | null
  start: (lang: string) => void
  stop: () => void
}

// useServerStt captures mic audio with MediaRecorder and, on stop, uploads the
// clip to the server (whisper.cpp) for transcription — the recognized text is
// pushed to onFinal. Unlike the browser Web Speech engine there is no interim
// preview: recognition happens once, after recording ends. getUserMedia still
// needs a secure context (localhost/HTTPS), so start() errors on plain LAN-HTTP.
export function useServerStt(onFinal: (text: string) => void): ServerStt {
  const [listening, setListening] = useState(false)
  const [transcribing, setTranscribing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const recRef = useRef<MediaRecorder | null>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const langRef = useRef('')
  const onFinalRef = useRef(onFinal)
  useEffect(() => {
    onFinalRef.current = onFinal
  }, [onFinal])

  const releaseStream = () => {
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
  }

  const start = useCallback(async (lang: string) => {
    setError(null)
    langRef.current = lang
    if (!navigator.mediaDevices?.getUserMedia) {
      setError('mikrofon erişimi yok (güvenli bağlam gerekir)')
      return
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream
      chunksRef.current = []
      const rec = new MediaRecorder(stream)
      rec.ondataavailable = (e) => {
        if (e.data.size > 0) chunksRef.current.push(e.data)
      }
      rec.onstop = async () => {
        releaseStream()
        const blob = new Blob(chunksRef.current, { type: rec.mimeType || 'audio/webm' })
        chunksRef.current = []
        if (blob.size === 0) {
          setTranscribing(false)
          return
        }
        setTranscribing(true)
        try {
          const text = await api.transcribe(blob, langRef.current, sttModel())
          const clean = text.trim()
          if (clean) onFinalRef.current(clean)
        } catch (e) {
          setError((e as Error).message)
        } finally {
          setTranscribing(false)
        }
      }
      recRef.current = rec
      rec.start()
      setListening(true)
    } catch (e) {
      // Permission denied, no device, or insecure context (LAN over HTTP).
      setError((e as Error).message)
      releaseStream()
    }
  }, [])

  const stop = useCallback(() => {
    const rec = recRef.current
    if (rec && rec.state !== 'inactive') rec.stop() // triggers onstop → transcribe
    setListening(false)
  }, [])

  // Tear down mic + recorder on unmount.
  useEffect(
    () => () => {
      const rec = recRef.current
      if (rec && rec.state !== 'inactive') rec.stop()
      releaseStream()
    },
    [],
  )

  return { listening, transcribing, error, start, stop }
}
