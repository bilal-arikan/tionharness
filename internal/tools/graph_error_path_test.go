package tools

import (
	"strings"
	"testing"
)

func TestGraphSchemaHint_JSONFieldPaths(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantParallel bool
	}{
		{"legacy", "Node.nodes.branches", true},
		{"indexed", "Graph.nodes.0.branches.0", true},
		{"multipleDigits", "Graph.nodes.12.branches.34", true},
		{"unrelatedField", "Graph.nodes.0.next", false},
		{"similarField", "Graph.nodes.0.branchesElse.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := "json: cannot unmarshal string into " + tt.path + " of type orchestration.Branch"
			hint := graphSchemaHint(msg)
			got := strings.HasPrefix(hint, "Fix: parallel fan-out")
			if got != tt.wantParallel {
				t.Fatalf("parallel hint = %v, want %v: %s", got, tt.wantParallel, hint)
			}
		})
	}
}
