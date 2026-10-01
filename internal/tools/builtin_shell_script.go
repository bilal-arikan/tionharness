package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Windows/MSYS command-line limits and quoting can cut a large heredoc before
// its closing delimiter. Run large Bash payloads from a complete script file;
// never feed the script on stdin, which belongs to commands inside it.
func prepareShellScript(cmd *exec.Cmd, label string) (func(), error) {
	noop := func() {}
	args := cmd.Args
	if label != "Bash" || len(args) < 3 || args[len(args)-2] != "-c" || len(args[len(args)-1]) < 8*1024 {
		return noop, nil
	}
	f, err := os.CreateTemp("", "tionharness-shell-*.sh")
	if err != nil {
		return noop, fmt.Errorf("prepare shell script: %w", err)
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	_, writeErr := f.WriteString(args[len(args)-1])
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		cleanup()
		if writeErr != nil {
			return noop, writeErr
		}
		return noop, closeErr
	}
	cmd.Args = append(args[:len(args)-2:len(args)-2], filepath.ToSlash(f.Name()))
	return cleanup, nil
}
