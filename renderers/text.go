package renderers

import (
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// IsArabic reports whether text contains at least one Arabic-script rune.
func IsArabic(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}

// NormalizeText applies NFC normalization before a renderer consumes text.
func NormalizeText(text string) string { return norm.NFC.String(text) }

// ShapeText is the format-neutral text preparation hook. The production PDF
// renderer performs contextual shaping in gopdf's TextElement/pdftext pipeline,
// where the active composite font is available; DOCX preserves logical text
// and delegates shaping to Word-compatible consumers.
func ShapeText(text string) string { return NormalizeText(text) }
