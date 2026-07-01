//go:build windows

package main

import (
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// DWM window attributes (dwmapi.h). Caption/text/border colors require Windows 11
// build 22000+; immersive dark mode works on Windows 10 1809+. Unsupported
// attributes simply return an error we ignore, so older Windows degrades to just
// the dark/light frame (or the default frame).
const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaBorderColor          = 34
	dwmwaCaptionColor         = 35
	dwmwaTextColor            = 36
)

var (
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// titleColors describes the native title-bar tint derived from an app theme.
type titleColors struct {
	dark    bool
	caption uint32 // COLORREF (0x00BBGGRR); ^uint32(0) sentinel = leave default
	text    uint32
	border  uint32
}

// Title-bar neutrals mirror frontend/src/lib/themePresets.ts (DARK_NEUTRALS /
// LIGHT_NEUTRALS). Every theme color shares these per-mode neutrals — only the
// accent differs, and the title bar doesn't use the accent — so the bar is
// decided purely by the preset id's "-light"/"-dark" suffix.
const (
	darkTitleBG, darkTitleText, darkTitleBorder    = "#0e0e10", "#e7e7ea", "#2b2b30"
	lightTitleBG, lightTitleText, lightTitleBorder = "#f5f6f8", "#16202c", "#d8dce3"
)

// resolveTitleColors maps the app appearance (preset id) to native title-bar
// colors. A "-light" preset yields the light neutrals; everything else (incl. an
// empty preset) yields the dark neutrals, matching the app's dark default.
func resolveTitleColors(preset, _ string) titleColors {
	if strings.HasSuffix(preset, "-light") {
		return titleColors{
			dark:    false,
			caption: hexToColorRef(lightTitleBG),
			text:    hexToColorRef(lightTitleText),
			border:  hexToColorRef(lightTitleBorder),
		}
	}
	return titleColors{
		dark:    true,
		caption: hexToColorRef(darkTitleBG),
		text:    hexToColorRef(darkTitleText),
		border:  hexToColorRef(darkTitleBorder),
	}
}

// applyTitleBar tints the native window chrome (caption, buttons, text, border)
// to match the app theme. Safe to call repeatedly and from any goroutine.
func applyTitleBar(hwnd uintptr, preset, theme string) {
	if hwnd == 0 {
		return
	}
	c := resolveTitleColors(preset, theme)

	darkVal := int32(0)
	if c.dark {
		darkVal = 1
	}
	setDWMAttr(hwnd, dwmwaUseImmersiveDarkMode, unsafe.Pointer(&darkVal), 4)

	if c.caption != ^uint32(0) {
		cap := c.caption
		setDWMAttr(hwnd, dwmwaCaptionColor, unsafe.Pointer(&cap), 4)
	}
	if c.text != ^uint32(0) {
		txt := c.text
		setDWMAttr(hwnd, dwmwaTextColor, unsafe.Pointer(&txt), 4)
	}
	if c.border != ^uint32(0) {
		brd := c.border
		setDWMAttr(hwnd, dwmwaBorderColor, unsafe.Pointer(&brd), 4)
	}
}

func setDWMAttr(hwnd uintptr, attr uint32, val unsafe.Pointer, size uintptr) {
	// Errors (e.g. attribute unsupported on this Windows build) are ignored by
	// design — the window still works, just with a less tailored frame.
	_, _, _ = procDwmSetWindowAttribute.Call(hwnd, uintptr(attr), uintptr(val), size)
}

// hexToColorRef converts "#RRGGBB" to a Win32 COLORREF (0x00BBGGRR). Invalid
// input yields the sentinel ^uint32(0) ("leave default").
func hexToColorRef(hex string) uint32 {
	s := strings.TrimPrefix(hex, "#")
	if len(s) != 6 {
		return ^uint32(0)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return ^uint32(0)
	}
	r := (v >> 16) & 0xff
	g := (v >> 8) & 0xff
	b := v & 0xff
	return uint32(b<<16 | g<<8 | r)
}
