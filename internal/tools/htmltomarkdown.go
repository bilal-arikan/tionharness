package tools

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

// htmltomarkdown.go is a dependency-free (stdlib-only) HTML→Markdown converter
// used by the WebFetch tool. It is intentionally pragmatic, not a full parser:
// a sequence of regexp passes drops non-content blocks, rewrites the common
// structural/inline tags to Markdown, strips whatever tags remain and unescapes
// entities. go.mod stays minimal (no x/net/html), matching the project's
// no-extra-deps convention.

var (
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// Non-content blocks removed wholesale (with their inner text).
	reDropBlocks = regexp.MustCompile(`(?is)<(script|style|noscript|template|svg|head|nav|footer|form|button|select|aside|iframe)\b[^>]*>.*?</(?:script|style|noscript|template|svg|head|nav|footer|form|button|select|aside|iframe)>`)
	rePre        = regexp.MustCompile(`(?is)<pre\b[^>]*>(.*?)</pre>`)
	reHeading    = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]>`)
	reAnchor     = regexp.MustCompile(`(?is)<a\b[^>]*?href\s*=\s*["']?([^"'>\s]+)["']?[^>]*>(.*?)</a>`)
	reListItem   = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	reBold       = regexp.MustCompile(`(?is)<(strong|b)\b[^>]*>(.*?)</(?:strong|b)>`)
	reItalic     = regexp.MustCompile(`(?is)<(em|i)\b[^>]*>(.*?)</(?:em|i)>`)
	reInlineCode = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code>`)
	reBlockClose = regexp.MustCompile(`(?is)</(p|div|section|article|header|main|tr|ul|ol|table|blockquote|h[1-6]|li)>`)
	reBreak      = regexp.MustCompile(`(?is)<br\s*/?>`)
	reAnyTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reTrailWS    = regexp.MustCompile(`[ \t]+\n`)
	reSpaces     = regexp.MustCompile(`[ \t]{2,}`)
	reManyNL     = regexp.MustCompile(`\n{3,}`)
)

// htmlToMarkdown converts an HTML document to a compact Markdown-ish text. base,
// when non-nil, is used to resolve relative anchor hrefs to absolute URLs.
func htmlToMarkdown(input string, base *url.URL) string {
	s := input
	s = reHTMLComment.ReplaceAllString(s, "")
	s = reDropBlocks.ReplaceAllString(s, "")

	// Fenced code first (preserve inner text, strip any inner tags later).
	s = rePre.ReplaceAllString(s, "\n\n```\n$1\n```\n\n")

	s = reHeading.ReplaceAllStringFunc(s, func(m string) string {
		sm := reHeading.FindStringSubmatch(m)
		level := len(sm[1]) // "1".."6" → 1..6 hashes (len of the digit string is 1)
		if n := int(sm[1][0] - '0'); n >= 1 && n <= 6 {
			level = n
		}
		return "\n\n" + strings.Repeat("#", level) + " " + strings.TrimSpace(sm[2]) + "\n\n"
	})

	s = reAnchor.ReplaceAllStringFunc(s, func(m string) string {
		sm := reAnchor.FindStringSubmatch(m)
		href, text := sm[1], strings.TrimSpace(sm[2])
		if base != nil {
			if ref, err := url.Parse(href); err == nil {
				href = base.ResolveReference(ref).String()
			}
		}
		if text == "" {
			text = href
		}
		return "[" + text + "](" + href + ")"
	})

	s = reInlineCode.ReplaceAllString(s, "`$1`")
	s = reBold.ReplaceAllString(s, "**$2**")
	s = reItalic.ReplaceAllString(s, "*$2*")
	s = reListItem.ReplaceAllString(s, "\n- $1")

	// Structural block closers and explicit breaks become newlines.
	s = reBlockClose.ReplaceAllString(s, "\n")
	s = reBreak.ReplaceAllString(s, "\n")

	// Drop every remaining tag, then unescape entities (&amp; &#39; …).
	s = reAnyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)

	// Whitespace cleanup: trim trailing spaces, collapse runs, cap blank lines.
	s = reTrailWS.ReplaceAllString(s, "\n")
	s = reSpaces.ReplaceAllString(s, " ")
	s = reManyNL.ReplaceAllString(s, "\n\n")

	// Trim each line's outer spaces without touching list/heading markers.
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
