package agent

import (
	"fmt"
)

// ErrUnknownIndexTool is returned when a drop names a tool whose index layout
// this package does not know. Deleting a directory guessed from an unknown
// tool's name is exactly the mistake the confirmation gate exists to prevent, so
// an unknown tool is refused rather than approximated.
var ErrUnknownIndexTool = fmt.Errorf("bu araç için indeks yönetimi tanımlı değil")

// DropSearchIndex deletes one managed index after the user confirmed it,
// dispatching on the tool.
//
// confirmRoot must repeat root exactly; see indexstate.Drop for why the
// confirmation is a path and not a flag. Only zvec-grep is droppable here:
// codebase-memory keeps its store in the user's cache directory under its own
// naming scheme, and removing that is the tool's own `delete_project` operation,
// not a directory this package should delete behind its back.
func (r *Runtime) DropSearchIndex(tool, root, confirmRoot string) error {
	switch tool {
	case exttoolsZvecGrepName:
		return r.DropZvecGrepIndex(root, confirmRoot)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownIndexTool, tool)
	}
}
