// Package notify renders and delivers the optional Telegram round
// notification. It is a pure side channel: it never changes file, ownership,
// counting or scheduling behavior, and it depends only on the Go standard
// library. The reconciler and storage packages provide structured facts; this
// package owns message formatting, the single bounded temporary attachment
// file and the official Bot API transport.
package notify

import (
	"fmt"
	"strings"
)

// Kind classifies one file-level event for the summary preview and the
// attachment listing. The values are fixed vocabulary.
type Kind string

const (
	KindCreated         Kind = "created"
	KindUpdated         Kind = "updated"
	KindDeleted         Kind = "deleted"
	KindReclaimed       Kind = "reclaimed"
	KindMissingCleared  Kind = "missing_cleared"
	KindMissingAdvanced Kind = "missing_advanced"
)

// Staged absence qualification reasons mirrored from the storage vocabulary.
const (
	ReasonSourceTarget      = "source_target"
	ReasonExcludedExtension = "excluded_extension"
)

// Event is one structured fact to record. RelativePath is the STRM path
// relative to its output root; it never contains a server path, database
// location, media URL or API response.
type Event struct {
	SourceIndex  int
	Kind         Kind
	RelativePath string
	PriorCount   int64
	NewCount     int64
	Reason       string
}

// bidiChars are the Unicode bidirectional control characters that can spoof
// the rendering order of a message. They are escaped like control
// characters so a crafted file name can never restructure the report.
var bidiChars = map[rune]bool{
	0x061C: true, // Arabic letter mark
	0x200E: true, // left-to-right mark
	0x200F: true, // right-to-left mark
	0x202A: true,
	0x202B: true,
	0x202C: true,
	0x202D: true,
	0x202E: true,
	0x2066: true,
	0x2067: true,
	0x2068: true,
	0x2069: true,
}

// EscapeText escapes control characters, line breaks and bidirectional control
// characters as \\uXXXX so a file name cannot inject new lines, fake events or
// reorder the rendered text. All other runes, including full Unicode file
// names and emoji, pass through unchanged.
func EscapeText(s string) string {
	if !needsEscape(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7F || bidiChars[r] {
			fmt.Fprintf(&b, `\u%04X`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func needsEscape(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7F || bidiChars[r] {
			return true
		}
	}
	return false
}

// UTF16Len reports the length of s in UTF-16 code units, the unit Telegram
// uses for message length limits.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// TruncateUTF16 cuts s to at most limit UTF-16 code units without splitting a
// Unicode character. The result is never longer than limit units.
func TruncateUTF16(s string, limit int) string {
	if UTF16Len(s) <= limit {
		return s
	}
	n := 0
	for i, r := range s {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if n+width > limit {
			return s[:i]
		}
		n += width
	}
	return s
}
