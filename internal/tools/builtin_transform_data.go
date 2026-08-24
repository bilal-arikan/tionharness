package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const (
	// transformDataTimeout bounds a script's wall-clock runtime.
	transformDataTimeout = 30 * time.Second
	// transformDataMaxScriptBytes caps the inline script size.
	transformDataMaxScriptBytes = 256 * 1024
	// transformDataMaxLogBytes caps the stdout/stderr we return for debugging —
	// the DATA never comes back through context, only the script's logs do.
	transformDataMaxLogBytes = 16 * 1024
	// transformDataMaxCountBytes caps how much of the output file we read back
	// solely to count rows (best-effort; larger files skip the count).
	transformDataMaxCountBytes = 16 * 1024 * 1024
)

// TransformDataTool runs a short script (python3/node/bun) in an isolated
// subprocess to RESHAPE data: the script reads input files and writes a
// structured JSON output file, which the agent then references (e.g. as a table's
// data source) WITHOUT inlining the rows into the conversation. This is the
// token-saving counterpart of pasting a large JSON blob into a reply.
//
// Safety: the subprocess runs with a STRIPPED environment (no API keys/secrets —
// see minimalScriptEnv), a 30s timeout, and capped log capture. It still executes
// arbitrary host code, so it is classified RiskExec and registered behind the
// same execution gate as the shell tool — it is NOT a security sandbox, only a
// secret-isolation + resource-bounding wrapper.
type TransformDataTool struct{ sb Sandbox }

// NewTransformDataTool binds the tool to the turn's working-dir sandbox: input
// and output paths resolve against it exactly like the filesystem tools.
func NewTransformDataTool(sb Sandbox) TransformDataTool { return TransformDataTool{sb: sb} }

func (TransformDataTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "transform_data",
		// Eager tool: the description carries only what the model cannot infer from the
		// schema — the argv contract, the must-write rule, and the "not a file editor"
		// guardrail. Everything else was prose the call itself teaches.
		Description: "Run a short script (python3, node or bun) in an isolated subprocess to transform data: " +
			"it reads your input files and writes a structured JSON output file, which you then reference as a " +
			"table/spreadsheet data source instead of inlining the rows into your reply.\n" +
			"argv: the input files in order, then the output file LAST (with no inputs, argv[1] IS the output). " +
			"Your script MUST write that file or the call fails even on exit 0 — only stdout/stderr plus the " +
			"output path, size and row count come back, never the data itself.\n" +
			"Stripped environment (no API keys/secrets), 30s timeout. NOT a file editor — to change source " +
			"files use Edit or apply_patch.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"language":{"type":"string","enum":["python3","node","bun"]},
				"script":{"type":"string","description":"Script source (see the argv contract)."},
				"input_files":{"type":"array","items":{"type":"string"},"description":"Input paths, relative to the working dir, passed as argv[1..N] in order."},
				"output_file":{"type":"string","description":"Path to write the JSON output, relative to the working dir, passed as the LAST argv."}
			},
			"required":["language","script","output_file"],
			"additionalProperties":false
		}`),
	}
}

func (t TransformDataTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Language   string   `json:"language"`
		Script     string   `json:"script"`
		InputFiles []string `json:"input_files"`
		OutputFile string   `json:"output_file"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.Script) == "" {
		return "", fmt.Errorf("script is required")
	}
	if len(args.Script) > transformDataMaxScriptBytes {
		return "", fmt.Errorf("script is too large (%d bytes, max %d)", len(args.Script), transformDataMaxScriptBytes)
	}
	if strings.TrimSpace(args.OutputFile) == "" {
		return "", fmt.Errorf("output_file is required")
	}
	if !t.sb.Ready() {
		return "", fmt.Errorf("no working directory is configured for transform_data")
	}

	interp, scriptExt, err := resolveInterpreter(args.Language)
	if err != nil {
		return "", err
	}

	// Resolve and verify every input file up front — a missing input is a hard
	// error, never silently skipped, so the script can't run on partial data.
	cmdArgs := make([]string, 0, len(args.InputFiles)+2)
	scriptPath, cleanup, err := writeTempScript(args.Script, scriptExt)
	if err != nil {
		return "", err
	}
	defer cleanup()
	cmdArgs = append(cmdArgs, scriptPath)

	for _, f := range args.InputFiles {
		abs, err := t.sb.Resolve(f)
		if err != nil {
			return "", fmt.Errorf("input file %q: %w", f, err)
		}
		if st, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("input file %q not found: %w", f, err)
		} else if st.IsDir() {
			return "", fmt.Errorf("input file %q is a directory", f)
		}
		cmdArgs = append(cmdArgs, abs)
	}

	outAbs, err := t.sb.Resolve(args.OutputFile)
	if err != nil {
		return "", fmt.Errorf("output_file %q: %w", args.OutputFile, err)
	}
	if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	// Remove any stale output so a script that fails to write can't be mistaken
	// for success by an old file lingering at the path.
	_ = os.Remove(outAbs)
	cmdArgs = append(cmdArgs, outAbs)

	runCtx, cancel := context.WithTimeout(ctx, transformDataTimeout)
	defer cancel()

	cmd := proc.CommandContext(runCtx, interp, cmdArgs...)
	// The script may itself spawn processes; on timeout they must die with it,
	// otherwise a survivor keeps the output pipes open and this call never ends.
	proc.TreeKill(cmd)
	cmd.Dir = t.sb.Root
	cmd.Env = minimalScriptEnv()
	var logBuf bytes.Buffer
	w := &capWriter{buf: &logBuf, max: transformDataMaxLogBytes}
	cmd.Stdout = w
	cmd.Stderr = w
	runErr := cmd.Run()

	logs := strings.TrimSpace(logBuf.String())
	if w.truncated {
		logs += "\n[logs truncated at 16KB]"
	}

	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("transform_data timed out after %s\n%s", transformDataTimeout, logs)
	}
	if runErr != nil {
		// Surface the script's own error output instead of swallowing it — a failed
		// transform must fail loudly so the model can fix the script.
		return "", fmt.Errorf("transform_data script failed (%v)\n%s", runErr, logs)
	}

	st, err := os.Stat(outAbs)
	if err != nil {
		return "", fmt.Errorf("script exited 0 but did not write the output file %q\n%s", args.OutputFile, logs)
	}

	return formatTransformResult(outAbs, st.Size(), logs), nil
}

// resolveInterpreter maps a language to a runnable interpreter on PATH and the
// temp-script extension to use. On Windows real CPython is installed as
// python.exe (python3.exe usually exists ONLY as a Microsoft Store "app execution
// alias" stub), so "python" is tried first there; bun runs the same .js the node
// path uses.
func resolveInterpreter(language string) (exePath, scriptExt string, err error) {
	switch language {
	case "python3":
		cands := proc.PythonCandidates()
		if p, ok := lookInterpreter(cands...); ok {
			return p, ".py", nil
		}
		return "", "", fmt.Errorf("no python interpreter found on PATH (tried %s)", strings.Join(cands, ", "))
	case "node":
		if p, ok := lookInterpreter("node"); ok {
			return p, ".js", nil
		}
		return "", "", fmt.Errorf("node not found on PATH")
	case "bun":
		if p, ok := lookInterpreter("bun"); ok {
			return p, ".js", nil
		}
		return "", "", fmt.Errorf("bun not found on PATH")
	default:
		return "", "", fmt.Errorf("unsupported language %q (use python3, node or bun)", language)
	}
}

// lookInterpreter resolves an interpreter on PATH. The rule (candidate order plus
// skipping Microsoft Store app-execution-alias stubs) lives in internal/proc so the
// external-tools catalog reports on exactly the binary this tool would run.
var lookInterpreter = proc.LookInterpreter

// writeTempScript writes src to a temp file with the given extension and returns
// its path plus a cleanup func. Running from a real file (vs. -c/-e) gives correct
// line numbers in tracebacks and sidesteps command-line length limits.
func writeTempScript(src, ext string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "tionharness-transform-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("create temp script: %w", err)
	}
	if _, err := f.WriteString(src); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("write temp script: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
}

// formatTransformResult builds the compact summary returned to the model: the
// output path (for use as a table src), its size, a best-effort row count, and
// any script logs. The data itself is intentionally NOT included.
func formatTransformResult(outAbs string, size int64, logs string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Wrote %s (%s).", outAbs, humanBytes(size))
	if n, ok := countRows(outAbs, size); ok {
		fmt.Fprintf(&b, " %d row(s).", n)
	}
	b.WriteString("\nReference it via a datatable/spreadsheet \"src\" instead of inlining the rows.")
	if logs != "" {
		fmt.Fprintf(&b, "\n\n--- script output ---\n%s", logs)
	}
	return b.String()
}

// countRows best-effort counts rows in the output file: it accepts either
// {"rows":[...]} or a bare top-level array [...]. Files above the cap are skipped
// (ok=false) to avoid reading a huge payload back into memory.
func countRows(path string, size int64) (int, bool) {
	if size > transformDataMaxCountBytes {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var wrapped struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Rows != nil {
		return len(wrapped.Rows), true
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		return len(arr), true
	}
	return 0, false
}

// humanBytes renders a byte size as B/KB/MB for the summary line.
func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// capWriter buffers up to max bytes and drops the rest, recording truncation.
type capWriter struct {
	buf       *bytes.Buffer
	max       int
	truncated bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) <= room {
			w.buf.Write(p)
		} else {
			w.buf.Write(p[:room])
			w.truncated = true
		}
	} else if len(p) > 0 {
		w.truncated = true
	}
	return len(p), nil
}
