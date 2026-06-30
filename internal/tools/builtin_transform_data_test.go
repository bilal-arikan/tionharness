package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pythonAvailable reports whether a python interpreter is on PATH so tests that
// actually run a script can skip cleanly on hosts without one.
func pythonAvailable() bool {
	_, _, err := resolveInterpreter("python3")
	return err == nil
}

func TestTransformDataUnsupportedLanguage(t *testing.T) {
	tool := NewTransformDataTool(NewSandbox(t.TempDir()))
	if _, err := tool.Call(t.Context(), []byte(`{"language":"ruby","script":"x","output_file":"o.json"}`)); err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestTransformDataRequiresScriptAndOutput(t *testing.T) {
	tool := NewTransformDataTool(NewSandbox(t.TempDir()))
	if _, err := tool.Call(t.Context(), []byte(`{"language":"python3","script":"  ","output_file":"o.json"}`)); err == nil {
		t.Fatal("expected error for blank script")
	}
	if _, err := tool.Call(t.Context(), []byte(`{"language":"python3","script":"print(1)","output_file":""}`)); err == nil {
		t.Fatal("expected error for blank output_file")
	}
}

func TestTransformDataMissingInputFile(t *testing.T) {
	tool := NewTransformDataTool(NewSandbox(t.TempDir()))
	in := `{"language":"python3","script":"print(1)","input_files":["nope.json"],"output_file":"o.json"}`
	if _, err := tool.Call(t.Context(), []byte(in)); err == nil {
		t.Fatal("expected error for missing input file (must not be silently skipped)")
	}
}

func TestTransformDataHappyPath(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.json"), []byte(`{"ids":[1,2,3,4]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewTransformDataTool(NewSandbox(dir))

	script := "import json,sys\n" +
		"data=json.load(open(sys.argv[1]))\n" +
		"rows=[{'id':x} for x in data['ids']]\n" +
		"json.dump({'rows':rows},open(sys.argv[2],'w'))\n" +
		"print('wrote',len(rows))\n"
	args, _ := json.Marshal(map[string]any{
		"language":    "python3",
		"script":      script,
		"input_files": []string{"in.json"},
		"output_file": "out/result.json",
	})

	out, err := tool.Call(t.Context(), args)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "4 row(s)") {
		t.Errorf("expected '4 row(s)' in summary, got:\n%s", out)
	}
	if !strings.Contains(out, "wrote 4") {
		t.Errorf("expected script stdout 'wrote 4' in summary, got:\n%s", out)
	}
	// The output file must exist with the rows; the data is NOT in the summary.
	written, err := os.ReadFile(filepath.Join(dir, "out", "result.json"))
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if !strings.Contains(string(written), `"id"`) {
		t.Errorf("output file missing rows: %s", written)
	}
}

func TestTransformDataScriptFailureSurfaces(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	tool := NewTransformDataTool(NewSandbox(t.TempDir()))
	// Script raises — the error and traceback must surface, not be swallowed.
	args, _ := json.Marshal(map[string]any{
		"language":    "python3",
		"script":      "raise SystemExit('boom-marker')",
		"output_file": "o.json",
	})
	_, err := tool.Call(t.Context(), args)
	if err == nil {
		t.Fatal("expected error when the script fails")
	}
	if !strings.Contains(err.Error(), "boom-marker") {
		t.Errorf("script error message must surface, got: %v", err)
	}
}

func TestTransformDataNoOutputWritten(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	tool := NewTransformDataTool(NewSandbox(t.TempDir()))
	args, _ := json.Marshal(map[string]any{
		"language":    "python3",
		"script":      "print('did nothing')",
		"output_file": "o.json",
	})
	if _, err := tool.Call(t.Context(), args); err == nil {
		t.Fatal("expected error when the script exits 0 without writing the output file")
	}
}

func TestTransformDataEnvStripped(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	// A secret in the PARENT env must NOT be visible to the script.
	t.Setenv("SWARMGO_SECRET_TEST", "leaked-value")
	dir := t.TempDir()
	tool := NewTransformDataTool(NewSandbox(dir))
	script := "import os,sys,json\n" +
		"json.dump({'seen':os.environ.get('SWARMGO_SECRET_TEST','ABSENT')},open(sys.argv[1],'w'))\n"
	args, _ := json.Marshal(map[string]any{
		"language":    "python3",
		"script":      script,
		"output_file": "o.json",
	})
	if _, err := tool.Call(t.Context(), args); err != nil {
		t.Fatalf("Call: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(dir, "o.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "leaked-value") {
		t.Errorf("parent-env secret leaked into the script: %s", written)
	}
	if !strings.Contains(string(written), "ABSENT") {
		t.Errorf("expected the secret to be ABSENT in the script env: %s", written)
	}
}

func TestMinimalScriptEnvDropsSecrets(t *testing.T) {
	t.Setenv("SWARMGO_SECRET_UNIT", "leak")
	env := minimalScriptEnv()
	var hasPath bool
	for _, kv := range env {
		if strings.HasPrefix(kv, "SWARMGO_SECRET_UNIT=") {
			t.Errorf("allowlisted env must not include the secret: %q", kv)
		}
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "Path=") {
			hasPath = true
		}
	}
	if !hasPath {
		t.Error("minimal env must pass PATH through so the interpreter is found")
	}
}
