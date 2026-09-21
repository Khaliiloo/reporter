package renderers

import (
	_ "embed"

	"github.com/go-pdf/fpdf"
)

// The DejaVu Sans (Condensed) family is embedded to give the PDF renderer
// Unicode support. It is licensed under the permissive Bitstream Vera font
// license and covers Latin, Cyrillic and Greek scripts. Complex-script text
// (Arabic, Devanagari) requires shaping that fpdf cannot provide; see the
// design notes for the recommended path.
//
//go:embed fonts/DejaVuSans.ttf
var dejaVuSans []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var dejaVuSansBold []byte

//go:embed fonts/DejaVuSans-Oblique.ttf
var dejaVuSansOblique []byte

//go:embed fonts/DejaVuSans-BoldOblique.ttf
var dejaVuSansBoldOblique []byte

// fontFamily is the fpdf family name registered for the embedded fonts.
const fontFamily = "DejaVu"

// registerUnicodeFonts registers the embedded Unicode font variants (regular,
// bold, oblique, bold-oblique) on a document so bold/italic styles render
// correctly. It must be called before any font is selected on pdf.
func registerUnicodeFonts(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes(fontFamily, "", dejaVuSans)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", dejaVuSansBold)
	pdf.AddUTF8FontFromBytes(fontFamily, "I", dejaVuSansOblique)
	pdf.AddUTF8FontFromBytes(fontFamily, "BI", dejaVuSansBoldOblique)
}
