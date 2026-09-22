package treepin

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// gitRaw runs git in dir and returns stdout untouched.
func gitRaw(dir string, args ...string) (string, error) {
	cmd := proc.Command("git", append([]string{"-c", "core.quotepath=false"}, args...)...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("treepin: git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// git runs git in dir and returns trimmed stdout.
func git(dir string, args ...string) (string, error) {
	out, err := gitRaw(dir, args...)
	return strings.TrimSpace(out), err
}
