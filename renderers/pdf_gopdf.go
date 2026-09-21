package renderers

import (
	"fmt"
	"strings"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
	gp "github.com/raceresult/gopdf"
	"github.com/raceresult/gopdf/pdf"
)

const (
	gpPageW  = 595.28
	gpPageH  = 841.89
	gpMargin = 28.35
)

// renderWithGopdf is the production PDF path. gopdf's TextElement applies
// pdftext.StringModifications, including Arabic contextual shaping and RTL
// handling, before writing glyphs from the embedded composite font.
func renderWithGopdf(report *models.Report, opts models.ExportOptions, hook pdfLayoutHook) ([]byte, error) {
	logo, err := decodeLogo(report.Logo)
	if err != nil {
		return nil, err
	}
	b := gp.New()
	font, err := b.NewCompositeFont(dejaVuSans)
	if err != nil {
		return nil, fmt.Errorf("load Unicode composite font: %w", err)
	}
	pageW, pageH := gpPageW, gpPageH
	landscape := opts.PageOrientation == models.OrientationLandscape
	if landscape {
		pageW, pageH = pageH, pageW
	}
	usableW := pageW - 2*gpMargin
	cols := renderColumns(report)
	widths := gopdfColumnWidths(cols, usableW, opts.AutoSizeColumns)

	pageNo := 1
	page := b.NewPage(gp.GetStandardPageSize(gp.PageSizeA4, landscape))
	y := gpMargin
	if logo != nil {
		img, e := b.NewImage(logo.data)
		if e != nil {
			return nil, fmt.Errorf("embed logo: %w", e)
		}
		w, h := 120.0, 58.0
		if float64(logo.width)/float64(logo.height) < w/h {
			w = h * float64(logo.width) / float64(logo.height)
		} else {
			h = w * float64(logo.height) / float64(logo.width)
		}
		x := usableW - w/2 - gpMargin
		page.AddElement(&gp.ImageBoxElement{Left: gp.Pt(x), Top: gp.Pt(y), Width: gp.Pt(w), Height: gp.Pt(h), Img: img, HorizontalAlign: gp.HorizontalAlignCenter, VerticalAlign: gp.VerticalAlignMiddle})
		if hook != nil {
			gpEmit(hook, "logo", x, y, x+w, y+h)
		}
		y += h + 20
	}
	if report.Title != "" {
		addGPText(page, report.Title, font, 22, gpMargin, y, usableW, gp.HorizontalAlignCenter, true, gpColor("#07449e"))
		if hook != nil {
			gpEmit(hook, "title", gpMargin, y, gpMargin+usableW, y+28)
		}
		y += 40
	}
	addGPText(page, report.Header.Left, font, 10, gpMargin, y, usableW/2, gp.HorizontalAlignLeft, false, gpColor("#555555"))
	addGPText(page, report.Header.Right, font, 10, gpMargin+usableW/2, y, usableW/2, gp.HorizontalAlignRight, false, gpColor("#555555"))
	if hook != nil {
		gpEmit(hook, "meta", gpMargin, y, gpMargin+usableW/2, y+14)
		gpEmit(hook, "meta", gpMargin+usableW/2, y, gpMargin+usableW, y+14)
	}
	y += 20
	page.AddElement(&gp.LineElement{X1: gp.Pt(gpMargin), Y1: gp.Pt(y), X2: gp.Pt(gpMargin + usableW), Y2: gp.Pt(y), LineWidth: gp.Pt(0.6), Color: gpColor("#888888")})
	if hook != nil {
		gpEmit(hook, "separator", gpMargin, y, gpMargin+usableW, y+1)
	}
	y += 15

	rowH := 24.0
	drawHeader := func() {
		x := gpMargin
		for ci, col := range cols {
			bg := col.Style.HeaderCellColor
			if bg == "" {
				bg = "#07449e"
			}
			page.AddElement(&gp.RectElement{Left: gp.Pt(x), Top: gp.Pt(y), Width: gp.Pt(widths[ci]), Height: gp.Pt(rowH), FillColor: gpColor(bg), LineColor: gpColor("#A5C6CD"), LineWidth: gp.Pt(0.4)})
			sz := col.Style.HeaderFontSize
			if sz <= 0 {
				sz = 12
			}
			addGPText(page, col.Header, font, float64(sz), x+3, y, widths[ci]-6, gp.HorizontalAlignCenter, col.Style.HeaderFontStyle == models.FontBold || col.Style.HeaderFontStyle == models.FontNormal, gpColor(col.Style.HeaderFontColor))
			if hook != nil {
				gpEmit(hook, "header", x, y, x+widths[ci], y+rowH)
			}
			x += widths[ci]
		}
		y += rowH + 3
	}
	drawHeader()
	it := NewRowIterator(report)
	row := 0
	for it.Next() {
		cells := renderCells(report, it.Row())
		if y+rowH > pageH-45 {
			addGPFooter(page, report, font, pageW, pageH, pageNo)
			pageNo++
			page = b.NewPage(gp.GetStandardPageSize(gp.PageSizeA4, landscape))
			y = gpMargin
			drawHeader()
		}
		x := gpMargin
		for ci, cell := range cells {
			col := cols[ci]
			bg := cell.CellColor
			if bg == "" {
				bg = col.Style.ColumnCellColor
			}
			if bg == "" && row%4 < 2 {
				bg = "#f1f7ff"
			}
			if bg != "" {
				page.AddElement(&gp.RectElement{Left: gp.Pt(x), Top: gp.Pt(y), Width: gp.Pt(widths[ci]), Height: gp.Pt(rowH), FillColor: gpColor(bg)})
			}
			sz := cell.FontSize
			if sz <= 0 {
				sz = col.Style.DataFontSize
			}
			if sz <= 0 {
				sz = 10
			}
			align := gp.HorizontalAlignLeft
			if report.TextDirection == models.TextDirectionRTL || cell.Type == models.DataTypeNumber {
				align = gp.HorizontalAlignRight
			}
			if cell.Type == models.DataTypeBoolean {
				align = gp.HorizontalAlignCenter
			}
			style := cell.FontStyle
			if style == models.FontNormal {
				style = col.Style.DataFontStyle
			}
			addGPText(page, cell.String(), font, float64(sz), x+3, y+5, widths[ci]-6, align, style == models.FontBold, gpColor(col.Style.DataFontColor))
			if hook != nil {
				gpEmit(hook, "cell", x, y, x+widths[ci], y+rowH)
			}
			x += widths[ci]
		}
		y += rowH
		row++
	}
	addGPFooter(page, report, font, pageW, pageH, pageNo)
	return b.Build()
}

func gopdfColumnWidths(cols []models.Column, total float64, auto bool) []float64 {
	if len(cols) == 0 {
		return nil
	}
	w := make([]float64, len(cols))
	sum := 0.0
	for i, c := range cols {
		if auto {
			w[i] = float64(len([]rune(c.Header)) + 2)
		} else {
			w[i] = 1
		}
		sum += w[i]
	}
	for i := range w {
		w[i] = total * w[i] / sum
	}
	return w
}

// Existing layout hooks use millimetres (the former fpdf unit); keep that
// diagnostic contract stable while the production engine works in points.
func gpEmit(hook pdfLayoutHook, kind string, x0, y0, x1, y1 float64) {
	if hook == nil {
		return
	}
	const ptToMM = 25.4 / 72.0
	hook(kind, x0*ptToMM, y0*ptToMM, x1*ptToMM, y1*ptToMM)
}

func addGPText(page *gp.Page, text string, font pdf.FontHandler, size, left, top, width float64, align gp.HorizontalAlign, bold bool, color gp.Color) {
	text = ShapeText(text)
	if strings.TrimSpace(text) == "" {
		return
	}
	page.AddElement(&gp.TextBoxElement{TextElement: gp.TextElement{TextChunk: gp.TextChunk{Text: text, Font: font, FontSize: size, Bold: bold, Color: color}, Left: gp.Pt(left), Top: gp.Pt(top), TextAlign: align, LineHeight: size * 1.25}, Width: gp.Pt(width), Height: gp.Pt(size * 1.6), VerticalAlign: gp.VerticalAlignMiddle})
}

func gpColor(s string) gp.Color {
	c, err := gp.ParseColor(s)
	if err != nil {
		return gp.ColorRGBBlack
	}
	return c
}

func addGPFooter(page *gp.Page, report *models.Report, font pdf.FontHandler, pageW, pageH float64, pageNo int) {
	y := pageH - 30
	addGPText(page, report.Footer.Left, font, 9, gpMargin, y, (pageW-2*gpMargin)/2, gp.HorizontalAlignLeft, false, gpColor("#666666"))
	addGPText(page, report.Footer.Right, font, 9, gpMargin+(pageW-2*gpMargin)/2, y, (pageW-2*gpMargin)/2, gp.HorizontalAlignRight, false, gpColor("#666666"))
	addGPText(page, fmt.Sprintf("Page %d", pageNo), font, 9, gpMargin+(pageW-2*gpMargin)/2-45, y, 90, gp.HorizontalAlignCenter, false, gpColor("#666666"))
}
