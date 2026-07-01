package api

import "net/http"

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
