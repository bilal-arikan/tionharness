package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
)

func TestParseFileWrite(t *testing.T) {
	cases := []struct {
		name, tool, input  string
		wantPath, wantBody string
		wantOK             bool
	}{
		{"native", "Write", `{"path":"out/report.md","content":"# Hi"}`, "out/report.md", "# Hi", true},
		{"cli", "Write", `{"file_path":"C:\\tmp\\data.csv","content":"a,b"}`, "C:\\tmp\\data.csv", "a,b", true},
		{"not a writer", "Read", `{"path":"x","content":"y"}`, "", "", false},
		{"empty content", "Write", `{"path":"x","content":""}`, "", "", false},
		{"empty path", "Write", `{"content":"y"}`, "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, body, ok := parseFileWrite(c.tool, json.RawMessage(c.input))
			if ok != c.wantOK || p != c.wantPath || body != c.wantBody {
				t.Fatalf("got (%q,%q,%v) want (%q,%q,%v)", p, body, ok, c.wantPath, c.wantBody, c.wantOK)
			}
		})
	}
}

func TestArtifactKindForPath(t *testing.T) {
	cases := []struct{ path, kind, lang string }{
		{"a/b/notes.md", db.ArtifactMarkdown, ""},
		{"page.html", db.ArtifactHTML, ""},
		{"data.csv", db.ArtifactText, ""},
		{"main.go", db.ArtifactCode, "go"},
		{"script.py", db.ArtifactCode, "python"},
		{"diagram.svg", db.ArtifactSVG, ""},
		{"noext", db.ArtifactText, ""},
	}
	for _, c := range cases {
		k, l := artifactKindForPath(c.path)
		if k != c.kind || l != c.lang {
			t.Errorf("%s → (%s,%s) want (%s,%s)", c.path, k, l, c.kind, c.lang)
		}
	}
}

func TestCaptureFileArtifacts_DedupByPath(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	s := &Server{logger: slog.Default()}

	step := func(tool, input string) agent.TurnStep {
		return agent.TurnStep{Kind: agent.StepTool, Tool: tool, Input: json.RawMessage(input)}
	}

	// First turn: agent writes a CSV via the native tool.
	s.captureFileArtifacts(ctx, database, "sess", "agent", []agent.TurnStep{
		step("Write", `{"path":"weather.csv","content":"il,sicaklik\nAdana,24"}`),
		step("Read", `{"path":"weather.csv"}`), // non-write → ignored
	})
	arts, _ := database.ListArtifacts(ctx, "sess")
	if len(arts) != 1 {
		t.Fatalf("want 1 artifact, got %d", len(arts))
	}
	if arts[0].Kind != db.ArtifactText || arts[0].Title != "weather.csv" || arts[0].SourcePath != "weather.csv" {
		t.Fatalf("unexpected artifact: %+v", arts[0])
	}

	// Second turn: same file rewritten → updates in place (no duplicate).
	s.captureFileArtifacts(ctx, database, "sess", "agent", []agent.TurnStep{
		step("Write", `{"path":"weather.csv","content":"il,sicaklik\nAdana,25"}`),
	})
	arts, _ = database.ListArtifacts(ctx, "sess")
	if len(arts) != 1 {
		t.Fatalf("want 1 artifact after rewrite, got %d", len(arts))
	}
	if arts[0].Content != "il,sicaklik\nAdana,25" {
		t.Fatalf("content not updated: %q", arts[0].Content)
	}

	// An errored write step must not be captured.
	s.captureFileArtifacts(ctx, database, "sess", "agent", []agent.TurnStep{
		{Kind: agent.StepTool, Tool: "Write", IsError: true, Input: json.RawMessage(`{"path":"bad.txt","content":"x"}`)},
	})
	arts, _ = database.ListArtifacts(ctx, "sess")
	if len(arts) != 1 {
		t.Fatalf("errored write should be skipped, got %d artifacts", len(arts))
	}
}
