package skills

import (
	"strings"
	"testing"
)

const ccSkill = `---
name: CC Demo
description: A Claude Code skill
allowed-tools:
  - Bash(git *)
  - Read
paths:
  - "**/*.go"
version: 2.0.0
license: Apache-2.0
context: fork
model: claude-opus
argument-hint: "<file>"
---
Do the work. Use $ARGUMENTS and run !` + "`echo hi`" + ` inline.
`

func TestMapCCSkillMapping(t *testing.T) {
	content, res := mapCCSkill(ccSkill, "https://github.com/x/y", true)

	// allowed-tools → always_allow (block list), paths carried, provenance set.
	for _, want := range []string{"name:", "always_allow:", "Bash(git *)", "paths:", "**/*.go",
		"version: 2.0.0", "source_url:", "github.com/x/y"} {
		if !strings.Contains(content, want) {
			t.Errorf("rendered SKILL.md missing %q:\n%s", want, content)
		}
	}
	// access: shared requested AND model-invocation not disabled → shared.
	if !strings.Contains(content, "access: shared") {
		t.Errorf("expected access: shared:\n%s", content)
	}
	// Unsupported features warned.
	joined := strings.Join(res.Warnings, " | ")
	for _, w := range []string{"context: fork", "model", "slash-command", "shell injection"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing warning about %q; got: %s", w, joined)
		}
	}
}

func TestMapCCSkillDisableInvocation(t *testing.T) {
	raw := "---\nname: Guarded\ndescription: d\ndisable-model-invocation: true\n---\nbody"
	content, res := mapCCSkill(raw, "", true)
	if strings.Contains(content, "access: shared") {
		t.Errorf("disable-model-invocation:true must NOT be shared:\n%s", content)
	}
	if len(res.Warnings) == 0 {
		t.Errorf("expected a warning about disable-model-invocation downgrade")
	}
}

func TestImportCCSkillWritesAndResolves(t *testing.T) {
	ws := t.TempDir()
	s := New("", ws)
	files := map[string][]byte{
		"reference.md":  []byte("ref"),
		"../escape.txt": []byte("nope"), // path traversal must be refused
		"SKILL.md":      []byte("must be ignored"),
	}
	sk, res, err := s.ImportCCSkill("", ccSkill, "https://github.com/x/y", files, true)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Slug != "cc-demo" {
		t.Errorf("slug: got %q want cc-demo", sk.Slug)
	}
	if sk.Version != "2.0.0" || sk.License != "Apache-2.0" || sk.SourceURL == "" {
		t.Errorf("provenance not parsed back: %+v", sk)
	}
	if len(sk.Paths) == 0 {
		t.Errorf("conditional paths not carried: %+v", sk)
	}
	if len(sk.AlwaysAllow) == 0 {
		t.Errorf("always_allow not carried: %+v", sk)
	}
	// only reference.md copied; SKILL.md + traversal refused.
	if len(res.Files) != 1 || res.Files[0] != "reference.md" {
		t.Errorf("bundled files: got %v want [reference.md]", res.Files)
	}
	// duplicate import fails.
	if _, _, err := s.ImportCCSkill("cc-demo", ccSkill, "", nil, true); err == nil {
		t.Errorf("expected duplicate-slug error")
	}
}
