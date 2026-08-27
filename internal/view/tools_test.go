package view

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func toolsFixture() ToolsInput {
	return ToolsInput{
		MCPServers: []db.MCPServer{
			{ID: "M1", Name: "linear", Transport: db.MCPTransportStdio, Command: "linear-mcp", Enabled: true},
			{ID: "M2", Name: "github", Transport: db.MCPTransportHTTP, URL: "https://api.example/mcp", Enabled: true},
			{ID: "M3", Name: "old-server", Transport: db.MCPTransportStdio, Command: "old", Enabled: false},
		},
		ToolConfig: db.WorkspaceToolConfig{DisabledTools: []string{"shell", "browser"}},
	}
}

func TestToolsHeaderCountsServersAndDisabled(t *testing.T) {
	v, err := ProjectTools(toolsFixture(), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, want := range []string{"TOOLS", "3 MCP sunucu (2 aktif)", "2 araç kapalı"} {
		if !strings.Contains(v.Header, want) {
			t.Errorf("header missing %q: %q", want, v.Header)
		}
	}
}

func TestToolsBodyListsServersEnabledFirst(t *testing.T) {
	v, err := ProjectTools(toolsFixture(), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	// The enabled http server prints its URL; the enabled stdio server its command.
	if !strings.Contains(txt, "https://api.example/mcp") || !strings.Contains(txt, "linear-mcp") {
		t.Errorf("server endpoints missing:\n%s", txt)
	}
	if !strings.Contains(txt, "● aktif") || !strings.Contains(txt, "○ kapalı") {
		t.Errorf("enabled/disabled markers missing:\n%s", txt)
	}
	if !strings.Contains(txt, "kapalı araçlar: shell, browser") {
		t.Errorf("disabled tools line missing:\n%s", txt)
	}
	// A disabled server must sort after the enabled ones.
	if strings.Index(txt, "old-server") < strings.Index(txt, "linear") {
		t.Errorf("disabled server sorted before enabled ones:\n%s", txt)
	}
}

func TestToolsCardCapsServersAndCountsElided(t *testing.T) {
	in := toolsFixture()
	for i := 0; i < toolsServerRows+4; i++ {
		in.MCPServers = append(in.MCPServers, db.MCPServer{
			ID: fmt.Sprintf("X%d", i), Name: fmt.Sprintf("srv%d", i),
			Transport: db.MCPTransportStdio, Command: "x", Enabled: true,
		})
	}
	v, err := ProjectTools(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Elided != len(in.MCPServers)-toolsServerRows || v.ElidedUnit != "MCP sunucu" {
		t.Errorf("elision wrong: elided=%d unit=%q servers=%d", v.Elided, v.ElidedUnit, len(in.MCPServers))
	}
}

func TestToolsEmptyIsExplicit(t *testing.T) {
	v, err := ProjectTools(ToolsInput{Now: time.Now()}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "yapılandırılmış MCP sunucusu yok") {
		t.Errorf("empty tools not stated:\n%s", v.Text())
	}
}

func TestToolsTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectTools(toolsFixture(), LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}
