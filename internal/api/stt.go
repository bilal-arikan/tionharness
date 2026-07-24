package api

import (
	"io"
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/stt"
)

// sttMaxAudioBytes caps an uploaded clip (~25 MB ≈ minutes of Opus) so a bad
// request can't exhaust memory.
const sttMaxAudioBytes = 25 << 20

// registerSTTRoutes wires the optional server-side speech-to-text engine
// (whisper.cpp CLI + ffmpeg). status lets the frontend choose the server engine
// vs the browser's Web Speech API; the transcribe endpoint returns recognized text.
func (s *Server) registerSTTRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stt/status", s.handleSTTStatus)
	mux.HandleFunc("POST /api/stt", s.handleSTTTranscribe)
}

type sttStatusDTO struct {
	Available bool        `json:"available"`
	Models    []stt.Model `json:"models"`
}

// handleSTTStatus reports whether whisper-cli + ffmpeg + a model are installed
// and the available models, so clients can offer server-side dictation.
func (s *Server) handleSTTStatus(w http.ResponseWriter, _ *http.Request) {
	models := stt.Models()
	if models == nil {
		models = []stt.Model{}
	}
	writeJSON(w, http.StatusOK, sttStatusDTO{Available: stt.Available(), Models: models})
}

// handleSTTTranscribe transcribes a raw audio body (any ffmpeg-decodable format,
// e.g. webm/opus from MediaRecorder). Language + model are query params. Returns
// {text}; 503 when the engine is unavailable or a step fails so the client can
// fall back to browser recognition.
//
// POST /api/stt?lang=tr-TR&model=ggml-small   body: audio bytes
func (s *Server) handleSTTTranscribe(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, sttMaxAudioBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "reading audio: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "audio body is required")
		return
	}
	lang := r.URL.Query().Get("lang")
	model := r.URL.Query().Get("model")
	text, err := stt.Transcribe(r.Context(), body, lang, model)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}
