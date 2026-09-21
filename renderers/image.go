package renderers

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/Khaliiloo/reporter/models"
	"golang.org/x/text/unicode/bidi"
)

// logoImage is decoded once per render so renderers can embed caller-provided
// bytes directly, without creating a temporary file.
type logoImage struct {
	data          []byte
	format        string
	width, height int
}

// visualText applies Unicode BiDi run ordering for PDF's text API. DOCX keeps
// logical text and uses OpenXML bidi properties, which is the format-native
// approach. Arabic glyph shaping still depends on the PDF font engine; the
// renderer validates and embeds the configured Unicode font rather than
// silently substituting a non-Arabic font.
func visualText(text string, direction models.TextDirection) string {
	if direction != models.TextDirectionRTL || text == "" {
		return text
	}
	var p bidi.Paragraph
	if _, err := p.SetString(text, bidi.DefaultDirection(bidi.RightToLeft)); err != nil {
		return text
	}
	o, err := p.Order()
	if err != nil {
		return text
	}
	var out []byte
	for i := 0; i < o.NumRuns(); i++ {
		run := o.Run(i)
		if run.Direction() == bidi.RightToLeft {
			out = bidi.AppendReverse(out, run.Bytes())
		} else {
			out = append(out, run.Bytes()...)
		}
	}
	return string(out)
}

func decodeLogo(data []byte) (*logoImage, error) {
	if len(data) == 0 {
		return nil, nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode logo: %w", err)
	}
	format = strings.ToLower(format)
	if format == "jpeg" {
		format = "jpg"
	}
	if format != "png" && format != "jpg" {
		return nil, fmt.Errorf("unsupported logo format %q; use PNG or JPEG", format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("logo has invalid dimensions")
	}
	return &logoImage{data: data, format: format, width: cfg.Width, height: cfg.Height}, nil
}
