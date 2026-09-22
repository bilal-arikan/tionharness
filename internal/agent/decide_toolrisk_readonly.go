package agent

import (
	"path/filepath"
	"regexp"
	"strings"
)

// readOnlyCommand reports whether a shell command is obviously harmless — every
// segment of it only reads, lists, builds or tests — so the tool-risk site can
// skip the decision model for it. A coding session runs dozens of such commands
// per turn; sending each one would add latency and cost for a foregone answer.
//
// It is deliberately conservative: anything it does not recognise (including
// every interpreter invocation, since "python -c" can do anything) is NOT read
// only and goes to the decision model. A false "not read-only" costs one fast
// call; a false "read-only" skips a safety check, so the list only holds
// commands whose every form in this list is side-effect free.
func readOnlyCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || writesThroughRedirect(cmd) || strings.ContainsAny(cmd, "`") || strings.Contains(cmd, "$(") {
		return false
	}
	for _, seg := range splitCommandSegments(cmd) {
		if !readOnlySegment(seg) {
			return false
		}
	}
	return true
}

// commandSeparators splits a command line into its pipeline/list segments.
var commandSeparators = regexp.MustCompile(`&&|\|\||;|\||\r?\n`)

func splitCommandSegments(cmd string) []string {
	var out []string
	for _, s := range commandSeparators.Split(cmd, -1) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// harmlessRedirect matches the redirections that write nowhere that matters.
var harmlessRedirect = regexp.MustCompile(`\d?>&\d|\d?>\s*/dev/null|\d?>\s*\$null|\d?>\s*NUL\b`)

// writesThroughRedirect reports an output redirection that writes a file.
func writesThroughRedirect(cmd string) bool {
	return strings.Contains(harmlessRedirect.ReplaceAllString(cmd, ""), ">")
}

// readOnlyHeads are commands with no side effects in any form listed here.
var readOnlyHeads = map[string]bool{
	"ls": true, "dir": true, "pwd": true, "cat": true, "head": true, "tail": true,
	"less": true, "more": true, "wc": true, "echo": true, "printf": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true,
	"which": true, "where": true, "type": true, "file": true, "stat": true,
	"du": true, "df": true, "tree": true, "date": true, "whoami": true,
	"hostname": true, "uname": true, "sort": true, "uniq": true, "cut": true,
	"diff": true, "cmp": true, "jq": true, "yq": true, "basename": true,
	"dirname": true, "realpath": true, "readlink": true, "nproc": true,
	"true": true, "false": true, "test": true, "command": true, "ps": true,
	"sleep": true, "tr": true, "column": true, "nl": true, "md5sum": true,
	"sha1sum": true, "sha256sum": true, "cd": true,
	// PowerShell read cmdlets (lower-cased).
	"get-childitem": true, "get-content": true, "select-string": true,
	"get-location": true, "test-path": true, "get-item": true, "get-process": true,
	"get-command": true, "measure-object": true, "write-output": true,
	"write-host": true, "select-object": true, "where-object": true, "sort-object": true,
	"format-table": true, "format-list": true, "get-date": true,
}

// readOnlySubcommands lists, per multi-verb CLI, the verbs that only read,
// build or test.
var readOnlySubcommands = map[string]map[string]bool{
	"git": {"status": true, "diff": true, "log": true, "show": true, "rev-parse": true,
		"ls-files": true, "blame": true, "describe": true, "shortlog": true,
		"grep": true, "merge-base": true, "cat-file": true, "ls-tree": true},
	"go":     {"test": true, "vet": true, "build": true, "list": true, "version": true, "doc": true},
	"cargo":  {"check": true, "test": true, "build": true, "clippy": true, "tree": true},
	"npm":    {"test": true, "ls": true, "list": true, "view": true, "outdated": true},
	"pnpm":   {"test": true, "ls": true, "list": true, "outdated": true},
	"yarn":   {"test": true, "list": true, "outdated": true},
	"docker": {"ps": true, "images": true, "logs": true, "inspect": true, "version": true},
	"kubectl": {"get": true, "describe": true, "logs": true, "version": true,
		"explain": true, "top": true},
	"gh": {"status": true},
}

// readOnlyNpmScripts are `npm run <script>` targets that conventionally only
// check or build.
var readOnlyNpmScripts = map[string]bool{
	"test": true, "lint": true, "build": true, "typecheck": true, "format:check": true, "check": true,
}

func readOnlySegment(seg string) bool {
	fields := strings.Fields(seg)
	// Skip leading VAR=value assignments.
	for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return true
	}
	head := strings.ToLower(filepath.Base(strings.ReplaceAll(fields[0], `\`, "/")))
	head = strings.TrimSuffix(head, ".exe")
	args := fields[1:]
	switch head {
	case "find":
		// find itself only lists; its action flags can delete or run anything.
		for _, a := range args {
			if a == "-delete" || strings.HasPrefix(a, "-exec") || strings.HasPrefix(a, "-ok") || a == "-fprint" {
				return false
			}
		}
		return true
	case "sed":
		for _, a := range args {
			if a == "-i" || strings.HasPrefix(a, "-i") || strings.HasPrefix(a, "--in-place") {
				return false
			}
		}
		return true
	case "gofmt":
		for _, a := range args {
			if a == "-w" {
				return false
			}
		}
		return true
	case "npm", "pnpm", "yarn":
		if len(args) >= 2 && args[0] == "run" {
			return readOnlyNpmScripts[args[1]]
		}
	}
	if readOnlyHeads[head] {
		return true
	}
	verbs, ok := readOnlySubcommands[head]
	if !ok || len(args) == 0 {
		return false
	}
	verb := firstNonFlag(args)
	if head == "git" && verb == "branch" {
		// Listing branches is read-only; -d/-D/-m/-c change them.
		for _, a := range args {
			if a == "-d" || a == "-D" || a == "-m" || a == "-M" || a == "-c" || a == "-C" || a == "--delete" || a == "--move" {
				return false
			}
		}
		return true
	}
	return verbs[verb]
}

// firstNonFlag returns the first argument that is not a flag ("" if none), so
// "git -C repo status" reads as "status". A flag's value ("-C repo") is
// skipped for the flags known to take one.
func firstNonFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "-n" || a == "--namespace" {
				i++
			}
			continue
		}
		return a
	}
	return ""
}
