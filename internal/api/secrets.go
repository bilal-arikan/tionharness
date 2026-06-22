package api

import (
	"errors"
	"net/http"

	"github.com/bilal-arikan/swarmgo/internal/secrets"
)

// setSecretReq is the create/update payload. Value is write-only; an empty value
// is rejected (use DELETE to remove a secret).
type setSecretReq struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// handleListSecrets returns the workspace's secrets as masked metadata (no values).
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	vault := ws(r).Secrets
	if vault == nil {
		writeJSON(w, http.StatusOK, []secrets.Meta{})
		return
	}
	writeJSON(w, http.StatusOK, vault.List())
}

// handleSetSecret creates or replaces a secret in the workspace vault.
func (s *Server) handleSetSecret(w http.ResponseWriter, r *http.Request) {
	var req setSecretReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	vault := ws(r).Secrets
	if vault == nil {
		writeError(w, http.StatusInternalServerError, "secret vault unavailable")
		return
	}
	meta, err := vault.Set(req.Name, req.Value, req.Description)
	if err != nil {
		if errors.Is(err, secrets.ErrInvalidName) || errors.Is(err, secrets.ErrInvalidValue) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("secret set", "workspace", ws(r).ID, "name", meta.Name)
	writeJSON(w, http.StatusOK, meta)
}

// handleRevealSecret returns the decrypted value of a single secret. This is an
// explicit, owner-initiated action (the Show/Copy button) distinct from the
// masked list — agents fetch values through the secret_get tool instead.
func (s *Server) handleRevealSecret(w http.ResponseWriter, r *http.Request) {
	vault := ws(r).Secrets
	if vault == nil {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	name := r.PathValue("name")
	value, ok := vault.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "value": value})
}

// handleDeleteSecret removes a secret by name.
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	vault := ws(r).Secrets
	if vault == nil {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	name := r.PathValue("name")
	if err := vault.Delete(name); err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			writeError(w, http.StatusNotFound, "secret not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("secret deleted", "workspace", ws(r).ID, "name", name)
	writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})
}
