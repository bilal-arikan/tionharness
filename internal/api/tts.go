package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/tts"
)

// registerTTSRoutes wires the optional server-side text-to-speech engine (Piper
// CLI). status lets the frontend decide whether to use the server engine or fall
// back to the browser's speechSynthesis; the synth endpoint returns WAV bytes.
func (s *Server) registerTTSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tts/status", s.handleTTSStatus)
	mux.HandleFunc("POST /api/tts", s.handleTTSSynthesize)
}

type ttsStatusDTO struct {
	Available bool        `json:"available"`
	Voices    []tts.Voice `json:"voices"`
}

// handleTTSStatus reports whether a Piper binary + voices are installed and the
// list of available voices, so clients (incl. phones) can offer the server engine.
func (s *Server) handleTTSStatus(w http.ResponseWriter, r *http.Request) {
	voices := tts.Voices()
	if voices == nil {
		voices = []tts.Voice{}
	}
	writeJSON(w, http.StatusOK, ttsStatusDTO{Available: tts.Available(), Voices: voices})
}

type ttsSynthReq struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

// handleTTSSynthesize renders the given text to WAV via Piper and streams it back
// as audio/wav. 503 when the engine is unavailable or the run fails, so the client
// can gracefully fall back to browser speech.
func (s *Server) handleTTSSynthesize(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[ttsSynthReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	data, err := tts.Synthesize(r.Context(), req.Text, req.Voice)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}
