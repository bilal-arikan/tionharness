package events

import "testing"

func TestWorkspaceStreamTypes(t *testing.T) {
	for _, typ := range []string{TypeWSSessionLifecycle, TypeWSTrajectory, TypeWSFlowRun, TypeWSScheduleArmed, TypeWSAutomationFire, TypeWSSpawn, TypeWSReport} {
		if !IsWorkspaceStream(typ) {
			t.Fatalf("%q must be a workspace-stream type", typ)
		}
		if kind := WorkspaceStreamKind(typ); kind == "" || kind == typ || IsWorkspaceStream(kind) {
			t.Fatalf("WorkspaceStreamKind(%q) = %q, want the bare kind", typ, kind)
		}
	}
	for _, typ := range []string{TypeChat, TypeFlow, TypeSessionStep, "", "ws:", "workspace"} {
		if IsWorkspaceStream(typ) {
			t.Fatalf("%q must not be a workspace-stream type", typ)
		}
		if WorkspaceStreamKind(typ) != typ {
			t.Fatalf("WorkspaceStreamKind(%q) must be identity", typ)
		}
	}
	if kind := WorkspaceStreamKind(TypeWSFlowRun); kind != "flow_run" {
		t.Fatalf("flow run kind = %q, want flow_run", kind)
	}
}
