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

// presetTitleColors mirrors frontend/src/lib/themePresets.ts (bg/text/border per
// curated palette). Kept small and in sync by hand — only the title bar needs
// these three tokens, not the full palette.
var presetTitleColors = map[string]struct {
	dark           bool
	bg, text, brdr string
}{
	"midnight-violet": {true, "#0c0c10", "#e7e7ea", "#2a2a33"},
	"slate":           {true, "#0d1117", "#e6edf3", "#30363d"},
	"emerald":         {true, "#0a0f0d", "#e6efe9", "#25332c"},
	"rose":            {true, "#100c0e", "#f0e7ea", "#33252d"},
	"amber":           {true, "#100d08", "#efe9df", "#332a1d"},
	"nord":            {true, "#242933", "#eceff4", "#434c5e"},
	"daylight":        {false, "#f6f8fb", "#16202c", "#d7dde6"},
	"solarized-light": {false, "#fdf6e3", "#073642", "#ddd6c1"},
}

// resolveTitleColors maps the app appearance (preset id, theme mode) to native
// title-bar colors. A known preset yields exact palette colors; otherwise we
// fall back to the legacy dark/light frame without a custom caption color.
func resolveTitleColors(preset, theme string) titleColors {
	if p, ok := presetTitleColors[preset]; ok {
		return titleColors{
			dark:    p.dark,
			caption: hexToColorRef(p.bg),
			text:    hexToColorRef(p.text),
			border:  hexToColorRef(p.brdr),
		}
	}
	// Legacy theme+accent: only know dark/light. "system" → treat as dark
	// (the app's default); a wrong guess only tints the frame, never breaks it.
	dark := theme != "light"
	return titleColors{dark: dark, caption: ^uint32(0), text: ^uint32(0), border: ^uint32(0)}
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
