package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// readJSONDoc decodes a JSON file into v with UseNumber, so numeric fields the
// repair does not touch survive the re-encode byte-for-byte. Without it every
// unix timestamp would come back as a float64 and be written out in exponent
// notation, silently corrupting rows this tool is only supposed to read past.
func readJSONDoc(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// writeJSONDoc writes v back to path atomically (temp file + rename), matching
// how the app itself persists its entity files.
func writeJSONDoc(path string, v any, indent bool) error {
	var (
		data []byte
		err  error
	)
	if indent {
		data, err = json.MarshalIndent(v, "", "  ")
	} else {
		data, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	tmp := path + ".repair-tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
