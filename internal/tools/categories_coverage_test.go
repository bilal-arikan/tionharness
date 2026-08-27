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

// declaredToolNames parses THIS package's sources and returns the Name of every
// built-in tool definition — i.e. the string literal in the providers.ToolDef
// composite literal each Tool's Def() method returns.
//
// Deriving the list from the definitions (and not from builtinCategory) is what
// makes the coverage test below non-tautological: a new built-in shows up here
// the moment its Def() exists, whether or not anyone remembered the map. It is
// also independent of Runtime.buildRegistry's per-session gating (shell, vault,
// coordination, skills), which would otherwise hide whole families of tools.
func declaredToolNames(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	names := map[string]string{} // tool name -> file it is declared in
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
			if n := toolDefName(fn.Body); n != "" {
				if prev, dup := names[n]; dup {
					t.Fatalf("tool name %q declared twice (%s and %s)", n, prev, name)
				}
				names[n] = filepath.Base(name)
			}
		}
	}
	if len(names) < 50 {
		t.Fatalf("only %d tool definitions found; the AST scan is broken", len(names))
	}
	return names
}

// toolDefName extracts the Name field of the providers.ToolDef literal returned
// by a Def() body. Returns "" when the body returns something else (e.g.
// funcTool, whose definition is supplied by its caller).
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
