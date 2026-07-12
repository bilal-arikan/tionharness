package api

import (
	"encoding/json"
	"testing"
)

// TestDecodeInbox_LegacyArray ensures a queue persisted in the old bare-array shape
// (written before the in-flight slot existed) still decodes after an upgrade, so an
// in-place update never strands a user's pending messages.
func TestDecodeInbox_LegacyArray(t *testing.T) {
	legacy := []inboxItem{
		{ClientMsgID: "a", WorkspaceID: "WS1"},
		{ClientMsgID: "b", WorkspaceID: "WS1"},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	pi := decodeInbox(data)
	if pi.Inflight != nil {
		t.Fatalf("legacy array must decode with no in-flight, got %+v", pi.Inflight)
	}
	if len(pi.Items) != 2 || pi.Items[0].ClientMsgID != "a" || pi.Items[1].ClientMsgID != "b" {
		t.Fatalf("legacy items not preserved: %+v", pi.Items)
	}
}

// TestDecodeInbox_ObjectWithInflight ensures the new object shape round-trips,
// keeping the interrupted in-flight head distinct from the waiting tail.
func TestDecodeInbox_ObjectWithInflight(t *testing.T) {
	src := persistedInbox{
		Inflight: &inboxItem{ClientMsgID: "head", Attempts: 2},
		Items:    []inboxItem{{ClientMsgID: "tail"}},
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	pi := decodeInbox(data)
	if pi.Inflight == nil || pi.Inflight.ClientMsgID != "head" || pi.Inflight.Attempts != 2 {
		t.Fatalf("in-flight head not preserved: %+v", pi.Inflight)
	}
	if len(pi.Items) != 1 || pi.Items[0].ClientMsgID != "tail" {
		t.Fatalf("waiting tail not preserved: %+v", pi.Items)
	}
}

// TestDecodeInbox_LeadingWhitespace ensures the shape sniff works even when the
// payload is pretty-printed / whitespace-prefixed.
func TestDecodeInbox_LeadingWhitespace(t *testing.T) {
	pi := decodeInbox([]byte("  \n\t[{\"clientMsgId\":\"x\"}]"))
	if pi.Inflight != nil || len(pi.Items) != 1 || pi.Items[0].ClientMsgID != "x" {
		t.Fatalf("whitespace-prefixed legacy array misparsed: %+v", pi)
	}
}

// TestDecodeInbox_Malformed decodes to an empty queue rather than panicking, so a
// corrupt sidecar never blocks boot.
func TestDecodeInbox_Malformed(t *testing.T) {
	pi := decodeInbox([]byte("not json"))
	if pi.Inflight != nil || len(pi.Items) != 0 {
		t.Fatalf("malformed payload should decode empty, got %+v", pi)
	}
}
