//go:build !windows

package tools

import "path/filepath"

// finalPath resolves path with filepath.EvalSymlinks. Outside Windows there are
// no junctions and EvalSymlinks follows every symlink, so it reports the same
// object an open would reach.
func finalPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
