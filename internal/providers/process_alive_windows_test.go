//go:build windows

package providers

import (
	"errors"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInvalidParameterUsesSupportedWindowsConstant(t *testing.T) {
	if !errors.Is(syscall.Errno(87), windows.ERROR_INVALID_PARAMETER) {
		t.Fatal("Windows error 87 must match windows.ERROR_INVALID_PARAMETER")
	}
}
