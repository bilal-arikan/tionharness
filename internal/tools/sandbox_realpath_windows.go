//go:build windows

package tools

import (
	"strings"

	"golang.org/x/sys/windows"
)

// GetFinalPathNameByHandle flags; the pinned x/sys predates their constants.
const (
	fileNameNormalized = 0x0 // FILE_NAME_NORMALIZED
	volumeNameDOS      = 0x0 // VOLUME_NAME_DOS
)

// finalPath opens path and asks the OS for the final path of the opened handle.
// Unlike filepath.EvalSymlinks, CreateFile follows junctions and every other
// name-surrogate reparse point, so the answer is the object the handle really
// refers to. Access 0 with full sharing opens files other processes hold open,
// and FILE_FLAG_BACKUP_SEMANTICS is required to open a directory at all.
func finalPath(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), fileNameNormalized|volumeNameDOS)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return stripFinalPathPrefix(windows.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n)
	}
}

// stripFinalPathPrefix turns GetFinalPathNameByHandle's \\?\ spelling back into
// the plain Win32 form Root is stored in (\\?\C:\x -> C:\x, \\?\UNC\s\x -> \\s\x).
func stripFinalPathPrefix(p string) string {
	if strings.HasPrefix(p, `\\?\UNC\`) {
		return `\\` + p[len(`\\?\UNC\`):]
	}
	return strings.TrimPrefix(p, `\\?\`)
}
