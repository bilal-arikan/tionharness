package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// indirectDefReceivers lists the receiver types whose Def() legitimately returns
// a definition built elsewhere instead of an inline providers.ToolDef literal.
// Every other Def() must yield a name, so a definition the scan cannot read is
// reported rather than skipped.
var indirectDefReceivers = map[string]bool{"funcTool": true}

// declaredToolNames parses THIS package's sources and returns the Name of every
// built-in tool definition — i.e. the string literal in the providers.ToolDef
// composite literal each Tool's Def() method returns.
//
// Deriving the list from the definitions (and not from builtinCategory) is what
// makes the coverage test below non-tautological: a new built-in shows up here
// the moment its Def() exists, whether or not anyone remembered the map. It is
// also independent of Runtime.buildRegistry's per-session gating (shell, vault,
// coordination, skills), which would otherwise hide whole families of tools.
//
// The scan is fail-loud by construction: a Def() whose name it cannot extract
// (a constant, a variable, a definition assembled at runtime) aborts the test
// instead of dropping the tool from the contract, and the number of names must
// account for every Def() method seen — so a broken scan cannot pass by finding
// "enough" tools.
func declaredToolNames(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	names := map[string]string{} // tool name -> file it is declared in
	defs := 0                    // Def() methods seen
	indirect := 0                // of those, allow-listed indirect ones
	var unreadable []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Def" || fn.Body == nil {
				continue
			}
			defs++
			recv := receiverTypeName(fn)
			if indirectDefReceivers[recv] {
				indirect++
				continue
			}
			n := toolDefName(fn.Body)
			if n == "" {
				unreadable = append(unreadable, filepath.Base(name)+": ("+recv+").Def()")
				continue
			}
			if prev, dup := names[n]; dup {
				t.Fatalf("tool name %q declared twice (%s and %s)", n, prev, name)
			}
			names[n] = filepath.Base(name)
		}
	}
	sort.Strings(unreadable)
	if len(unreadable) > 0 {
		t.Fatalf("%d Def() method(s) whose tool name the AST scan could not read — give Name a plain string literal, or add the receiver to indirectDefReceivers:\n  %s",
			len(unreadable), strings.Join(unreadable, "\n  "))
	}
	if want := defs - indirect; len(names) != want {
		t.Fatalf("scan extracted %d tool name(s) from %d Def() method(s) (%d allow-listed as indirect); expected %d — the AST scan is broken",
			len(names), defs, indirect, want)
	}
	return names
}

// receiverTypeName returns the bare type name of a method's receiver, so a Def()
// the scan cannot read can be named in the failure message.
func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "?"
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}

// toolDefName extracts the Name field of the providers.ToolDef literal returned
// by a Def() body. Returns "" when the body has no such literal, or when its
// Name is not a plain string literal — both are reported by the caller.
func toolDefName(body *ast.BlockStmt) string {
	var found string
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ToolDef" {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Name" {
				continue
			}
			str, ok := kv.Value.(*ast.BasicLit)
			if !ok || str.Kind != token.STRING {
				continue
			}
			v, err := strconv.Unquote(str.Value)
			if err == nil && v != "" {
				found = v
			}
		}
		return false
	})
	return found
}

// TestEveryBuiltinToolHasACategory enforces the contract stated at the top of
// categories.go: builtinCategory is the single source of truth, so every
// declared built-in must be listed there. A new tool whose name is not added to
// the map fails here instead of silently landing in "other" in the UI.
func TestEveryBuiltinToolHasACategory(t *testing.T) {
	declared := declaredToolNames(t)
	var missing []string
	for name := range declared {
		if _, ok := builtinCategory[name]; !ok {
			missing = append(missing, name+" ("+declared[name]+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("%d built-in tool(s) missing from builtinCategory (they fall into %q in the UI):\n  %s",
			len(missing), CategoryOther, strings.Join(missing, "\n  "))
	}
}

// TestNoStaleCategoryEntries is the other direction: a map entry naming a tool
// that no longer exists is dead weight and hides renames.
func TestNoStaleCategoryEntries(t *testing.T) {
	declared := declaredToolNames(t)
	var stale []string
	for name := range builtinCategory {
		if _, ok := declared[name]; !ok {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("%d builtinCategory entry/entries name no declared tool: %s",
			len(stale), strings.Join(stale, ", "))
	}
}
