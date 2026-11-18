// Package htmlrender owns the /render and /preview capabilities: agent-
// authored HTML pages that render inline in the transcript. Pages are plain
// workspace files; this package validates them, injects the theme/bridge
// bootstrap at serve time, and carries the artifact on the tool result.
package htmlrender

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxInlineBytes caps the html argument of render/preview.
	MaxInlineBytes = 512 * 1024
	// MaxFileBytes caps a page read from disk.
	MaxFileBytes = 4 * 1024 * 1024
	// MaxTitleRunes caps the card title.
	MaxTitleRunes = 200
)

var (
	htmlExtRe   = regexp.MustCompile(`(?i)\.html?$`)
	docStartRe  = regexp.MustCompile(`^\s*<(?:!doctype\s+html|html)[\s>]`)
	viewportRe  = regexp.MustCompile(`(?i)(?:\d+(?:\.\d+)?|\.\d+)[sld]?vh\b|(?:\d+(?:\.\d+)?|\.\d+)[sld]?vw\b`)
	cssSourceRe = regexp.MustCompile(`(?s)<!--[\s\S]*?(?:-->|$)|<style\b[^>]*(?:>|$)([\s\S]*?)(?:</style\s*>|$)|\sstyle\s*=\s*(?:"([^"]*)(?:"|$)|'([^']*)(?:'|$)|([^\s>]+))`)
)

// IsHTMLPath reports whether path names an HTML document.
func IsHTMLPath(path string) bool { return htmlExtRe.MatchString(path) }

// LooksLikeDocument reports whether content starts like an HTML document.
// Renamed non-HTML files (a .env saved as .html) fail this check.
func LooksLikeDocument(content string) bool { return docStartRe.MatchString(content) }

// RejectViewportUnits fails when CSS uses viewport-relative units. The card
// auto-fits its height to the page, so vh/vw would make it grow forever.
func RejectViewportUnits(html string) error {
	for _, m := range cssSourceRe.FindAllStringSubmatch(html, -1) {
		css := m[1]
		if css == "" {
			css = m[2]
		}
		if css == "" {
			css = m[3]
		}
		if css == "" {
			css = m[4]
		}
		if found := viewportRe.FindString(css); found != "" {
			return fmt.Errorf("page uses viewport unit %q; the frame fits the page height, so use fixed pixel heights or let content set the height", found)
		}
	}
	return nil
}

// blankNonMarkup replaces the contents of script/style/template/etc. regions
// with spaces so markup injection never lands inside a string literal. Same
// approach as t3code's blankNonMarkup: offsets are preserved.
var nonMarkupRe = regexp.MustCompile(`(?is)<!--[\s\S]*?(?:-->|$)|<script\b[\s\S]*?(?:</script\s*>|$)|<style\b[\s\S]*?(?:</style\s*>|$)|<textarea\b[\s\S]*?(?:</textarea\s*>|$)|<title\b[\s\S]*?(?:</title\s*>|$)|<xmp\b[\s\S]*?(?:</xmp\s*>|$)|<iframe\b[\s\S]*?(?:</iframe\s*>|$)|<noembed\b[\s\S]*?(?:</noembed\s*>|$)|<noframes\b[\s\S]*?(?:</noframes\s*>|$)|<noscript\b[\s\S]*?(?:</noscript\s*>|$)|<template\b[\s\S]*?(?:</template\s*>|$)|<plaintext\b[\s\S]*$`)

func blankNonMarkup(html string) string {
	return nonMarkupRe.ReplaceAllStringFunc(html, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

// InjectBootstrap inserts the theme/bridge markup into <head> (creating one
// when absent). The scan runs on blanked markup so offsets match the original.
func InjectBootstrap(html, markup string) string {
	scan := blankNonMarkup(html)
	headOpen := regexp.MustCompile(`(?i)<head(?:\s(?:[^"'>]|"[^"]*"|'[^']*')*)?>`).FindStringIndex(scan)
	if headOpen != nil {
		at := headOpen[1]
		return html[:at] + markup + html[at:]
	}
	htmlOpen := regexp.MustCompile(`(?i)<html(?:\s(?:[^"'>]|"[^"]*"|'[^']*')*)?>`).FindStringIndex(scan)
	if htmlOpen != nil {
		at := htmlOpen[1]
		return html[:at] + "<head>" + markup + "</head>" + html[at:]
	}
	doctype := regexp.MustCompile(`(?i)^\s*<!doctype[^>]*>`).FindStringIndex(html)
	if doctype != nil {
		at := doctype[1]
		return html[:at] + "<head>" + markup + "</head>" + html[at:]
	}
	return "<!doctype html><head>" + markup + "</head>" + html
}
