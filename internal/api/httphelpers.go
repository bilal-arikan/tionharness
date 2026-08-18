package api

import (
	"net/http"
	"strings"
)

// bindJSON decodes the request body into a fresh T. On failure it writes a 400
// "invalid JSON" response and returns ok=false, letting handlers collapse the
// repeated decode-and-error branch into:
//
//	req, ok := bindJSON[createFooReq](w, r)
//	if !ok {
//		return
//	}
//
// This is the read-side companion of writeJSON/writeDBError: those centralise the
// response path, this centralises the request-decode path.
func bindJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := decodeJSON(r, &v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return v, false
	}
	return v, true
}

// misplacedFieldHints maps a JSON field name that does not belong on the target
// struct to a human-readable explanation of where it actually belongs. Used by
// bindJSONStrict to turn a raw "unknown field" error into an actionable one.
var misplacedFieldHints = map[string]string{
	"workingDir": "working directory is a session property; set it when creating the session (POST /api/sessions), not on the agent",
}

// bindJSONStrict is bindJSON with unknown-field rejection: any JSON key that does
// not map to a field on T produces a 400 instead of being silently dropped. Use
// this for endpoints where a dropped field can misconfigure the created/updated
// resource in a way that only surfaces much later (see workingDir on
// POST/PUT /api/agents — a Session field, not an Agent one).
//
// Do not switch bindJSON itself to strict decoding: it is shared by ~40 handlers
// across the API, many of which are not audited for extra fields real clients may
// send (e.g. echoing a larger object back). Opt in per-endpoint instead.
func bindJSONStrict[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := decodeJSONStrict(r, &v); err != nil {
		msg := err.Error()
		if field, ok := strictUnknownField(msg); ok {
			if hint, ok := misplacedFieldHints[field]; ok {
				writeError(w, http.StatusBadRequest, "unknown field \""+field+"\" — "+hint)
				return v, false
			}
		}
		writeError(w, http.StatusBadRequest, "invalid JSON: "+msg)
		return v, false
	}
	return v, true
}

// strictUnknownField extracts the field name from the error message
// encoding/json's DisallowUnknownFields produces: `json: unknown field "x"`.
func strictUnknownField(msg string) (string, bool) {
	const marker = `unknown field "`
	i := strings.Index(msg, marker)
	if i < 0 {
		return "", false
	}
	rest := msg[i+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// requireFields writes a 400 "<name> is required" and returns false as soon as it
// finds a blank required value, so handlers can validate several fields in one
// call:
//
//	if !requireFields(w, "name", req.Name, "provider", req.Provider) {
//		return
//	}
//
// Arguments are (label, value) pairs; an odd trailing argument is ignored.
func requireFields(w http.ResponseWriter, pairs ...string) bool {
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] == "" {
			writeError(w, http.StatusBadRequest, pairs[i]+" is required")
			return false
		}
	}
	return true
}
