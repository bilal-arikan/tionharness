package mcp

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSchemaStripsMetaKeys(t *testing.T) {
	in := json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id": "urn:x",
		"type": "object",
		"additionalProperties": false,
		"required": ["q"],
		"properties": {
			"q": { "type": "string" },
			"opts": { "$schema": "nested", "oneOf": [ { "type": "string" } ] }
		}
	}`)
	out := NormalizeSchema(in)

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if _, ok := m["$schema"]; ok {
		t.Error("$schema not stripped at root")
	}
	if _, ok := m["$id"]; ok {
		t.Error("$id not stripped at root")
	}
	// Preserved keywords.
	if m["additionalProperties"] != false {
		t.Error("additionalProperties not preserved")
	}
	if _, ok := m["required"]; !ok {
		t.Error("required not preserved")
	}
	// Nested $schema stripped; oneOf preserved.
	props := m["properties"].(map[string]any)
	opts := props["opts"].(map[string]any)
	if _, ok := opts["$schema"]; ok {
		t.Error("nested $schema not stripped")
	}
	if _, ok := opts["oneOf"]; !ok {
		t.Error("oneOf union keyword not preserved")
	}
}

func TestNormalizeSchemaDefaultsRootObject(t *testing.T) {
	// Empty / missing type → permissive object schema.
	for _, in := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"properties":{}}`)} {
		out := NormalizeSchema(in)
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("invalid output for %s: %v", in, err)
		}
		if m["type"] != "object" {
			t.Errorf("expected root type=object for %s, got %v", in, m["type"])
		}
		if _, ok := m["properties"]; !ok {
			t.Errorf("expected properties present for %s", in)
		}
	}
}

func TestNormalizeSchemaInvalidFallback(t *testing.T) {
	out := NormalizeSchema(json.RawMessage(`not json`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil || m["type"] != "object" {
		t.Fatalf("invalid input should fall back to empty object, got %s", out)
	}
}
