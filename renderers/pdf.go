package renderers

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/models"
	"github.com/go-pdf/fpdf"
)

const (
	pdfMarginLeft   = 10.0
	pdfMarginRight  = 10.0
	pdfMarginTop    = 15.0
	pdfMarginBottom = 12.0
	pdfFooterZone   = 15.0

	pdfCellPadH  = 1.5
	pdfCellPadV  = 1.0
	pdfMinRowH   = 8.0
	pdfTitleSize = 16
	pdfMetaSize  = 10
	pdfLineLead  = 1.35
)

const (
	pdfBusinessTitleSize  = 22
	pdfBusinessHeaderFill = "#07449e"
)

// pdfLayoutHook records drawn layout elements for regression testing. It is
// nil in production and only set by tests.
type pdfLayoutHook func(kind string, x0, y0, x1, y1 float64)

// PDFRenderer renders reports to PDF with a deterministic grid-based table
// layout engine: the title and report metadata are rendered in the document
// body (not as page headers), followed by a bordered table with word-wrapped
// cells, pagination and a footer area.
type PDFRenderer struct {
	opts       models.ExportOptions
	layoutHook pdfLayoutHook
}

// NewPDFRenderer builds a PDFRenderer with the given export options.
func NewPDFRenderer(opts models.ExportOptions) *PDFRenderer {
	return &PDFRenderer{opts: opts}
}

// Render implements interfaces.Renderer.
func (r *PDFRenderer) Render(report *models.Report) ([]byte, error) {
	if err := checkConsistent(report); err != nil {
		return nil, errors.WrapRenderError(string(models.FormatPDF), err)
	}
	b, err := renderWithGopdf(report, r.opts, r.layoutHook)
	if err != nil {
		return nil, errors.WrapRenderError(string(models.FormatPDF),
			errors.WrapFormatError(string(models.FormatPDF), err))
	}
	return b, nil
}

// pdfDoc holds the layout state for a single render. It is created fresh per
// Render call so PDFRenderer instances remain safe for concurrent use.
type pdfDoc struct {
	pdf        *fpdf.Fpdf
	report     *models.Report
	opts       models.ExportOptions
	logo       *logoImage
	hook       pdfLayoutHook
	pageW      float64
	pageH      float64
	usableW    float64
	pageBreakY float64
	colStarts  []float64
	colWidths  []float64
	headerSize float64
	dataSize   float64
	columns    []models.Column
}

func (d *pdfDoc) initLayout() {
	d.pdf.SetMargins(pdfMarginLeft, pdfMarginTop, pdfMarginRight)
	d.pageW, d.pageH = d.pdf.GetPageSize()
	d.usableW = d.pageW - pdfMarginLeft - pdfMarginRight
	d.pageBreakY = d.pageH - pdfMarginBottom - pdfFooterZone

	d.headerSize = 12
	d.dataSize = 10
	d.columns = renderColumns(d.report)
	d.computeColumnWidths()
}

func renderColumns(report *models.Report) []models.Column {
	cols := append([]models.Column(nil), report.Columns...)
	if report.TextDirection == models.TextDirectionRTL {
		for i, j := 0, len(cols)-1; i < j; i, j = i+1, j-1 {
			cols[i], cols[j] = cols[j], cols[i]
		}
	}
	return cols
}

func renderCells(report *models.Report, cells []models.Cell) []models.Cell {
	if report.TextDirection != models.TextDirectionRTL {
		return cells
	}
	out := make([]models.Cell, len(cells))
	for i := range cells {
		out[len(cells)-1-i] = cells[i]
	}
	return out
}

// computeColumnWidths assigns every column a fixed width so header and data
// cells share the same X positions. Widths are proportional to content length
// when auto-sizing is enabled, equal otherwise. The result always fits the
// usable page width.
func (d *pdfDoc) computeColumnWidths() {
	n := len(d.columns)
	if n == 0 {
		return
	}
	d.colStarts = make([]float64, n)
	d.colWidths = make([]float64, n)

	weights := make([]float64, n)
	for ci, col := range d.columns {
		if !d.opts.AutoSizeColumns {
			weights[ci] = 1
			continue
		}
		units := float64(len([]rune(col.Header)))
		for ri := range col.ColumnData {
			if u := float64(len([]rune(col.ColumnData[ri].String()))); u > units {
				units = u
			}
		}
		weights[ci] = units + 2
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		total = 1
	}

	x := pdfMarginLeft
	widths := make([]float64, n)
	for ci, w := range weights {
		widths[ci] = d.usableW * w / total
		d.colStarts[ci] = x
		x += widths[ci]
	}

	// Guard against rounding drift and minimum-width overflow.
	if widths[n-1] < 1 {
		widths[n-1] = 1
	}
	for ci := range widths {
		if widths[ci] < 8 {
			// Shrink other columns to keep the grid within the page.
			excess := 8 - widths[ci]
			widths[ci] = 8
			for j := range widths {
				if j != ci && widths[j] > 12 {
					share := excess * widths[j] / (d.usableW - 8*float64(n))
					if widths[j]-share >= 8 {
						widths[j] -= share
						excess -= share
					}
					if excess <= 0.01 {
						break
					}
				}
			}
		}
	}

	// Recompute starts from final widths.
	x = pdfMarginLeft
	for ci, w := range widths {
		d.colWidths[ci] = w
		d.colStarts[ci] = x
		x += w
	}
}

func (d *pdfDoc) lineHeight(size float64) float64 {
	h := size * 0.3528 * pdfLineLead
	if h < 4.0 {
		h = 4.0
	}
	return h
}

// drawTitleAndMeta renders the report title (centered, first) followed by the
// header_left/header_right metadata on one line and a separator rule below.
func (d *pdfDoc) drawTitleAndMeta() {
	y := d.pdf.GetY()
	if d.logo != nil {
		const maxW, maxH = 52.0, 22.0
		w := maxW
		h := w * float64(d.logo.height) / float64(d.logo.width)
		if h > maxH {
			h = maxH
			w = h * float64(d.logo.width) / float64(d.logo.height)
		}
		name := "report-logo"
		d.pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: d.logo.format}, bytes.NewReader(d.logo.data))
		x := pdfMarginLeft + (d.usableW-w)*0.9
		d.pdf.ImageOptions(name, x, y, w, h, false, fpdf.ImageOptions{ImageType: d.logo.format}, 0, "")
		d.emit("logo", x, y, x+w, y+h)
		d.pdf.SetY(y + h + 5)
		y = d.pdf.GetY()
	}

	if d.report.Title != "" {
		d.pdf.SetFont(fontFamily, "B", pdfBusinessTitleSize)
		d.pdf.SetTextColor(0x13, 0x4E, 0x5E)
		d.pdf.SetX(pdfMarginLeft)
		d.pdf.CellFormat(d.usableW, 14, d.report.Title, "", 2, "C", false, 0, "")
		d.emit("title", pdfMarginLeft, y, pdfMarginLeft+d.usableW, y+14)
		d.pdf.Ln(5)
	}

	// Report metadata line: header_left at left, header_right at right.
	y = d.pdf.GetY()
	d.pdf.SetFont(fontFamily, "", pdfMetaSize)
	d.pdf.SetTextColor(0x55, 0x55, 0x55)
	half := d.usableW / 2
	d.pdf.SetX(pdfMarginLeft)
	d.pdf.CellFormat(half, 6, d.report.Header.Left, "", 0, "L", false, 0, "")
	d.pdf.CellFormat(half, 6, d.report.Header.Right, "", 0, "R", false, 0, "")
	d.emit("meta", pdfMarginLeft, y, pdfMarginLeft+half, y+6)
	d.emit("meta", pdfMarginLeft+half, y, pdfMarginLeft+d.usableW, y+6)
	d.pdf.Ln(6)

	// Separator rule.
	y = d.pdf.GetY() + 2
	d.pdf.SetDrawColor(0x88, 0x88, 0x88)
	d.pdf.SetLineWidth(0.4)
	d.pdf.Line(pdfMarginLeft, y, pdfMarginLeft+d.usableW, y)
	d.emit("separator", pdfMarginLeft, y, pdfMarginLeft+d.usableW, y+0.4)
	d.pdf.Ln(6)
}

// drawTable draws the header row and all data rows using the shared grid,
// repeating the header row after page breaks.
func (d *pdfDoc) drawTable() {
	d.drawTableHeader()

	it := NewRowIterator(d.report)
	row := 0
	for it.Next() {
		cells := renderCells(d.report, it.Row())
		rh := d.rowHeight(cells)
		maxH := d.pageBreakY - pdfMarginTop
		if rh > maxH {
			// A row taller than a full page: draw it anyway to avoid an
			// infinite page-break loop.
			rh = maxH
		}
		if d.pdf.GetY()+rh > d.pageBreakY {
			d.pdf.AddPage()
			d.drawTableHeader()
		}
		d.drawRow(cells, rh, row%2 == 1)
		row++
	}
}

// drawTableHeader draws every column header in a single row at the same Y.
func (d *pdfDoc) drawTableHeader() {
	y := d.pdf.GetY()
	hh := pdfMinRowH
	for _, col := range d.columns {
		size := col.Style.HeaderFontSize
		if size <= 0 {
			size = int(d.headerSize)
		}
		if h := float64(size)*0.3528*pdfLineLead + 2*pdfCellPadV; h > hh {
			hh = h
		}
	}
	for ci, col := range d.columns {
		style := string(col.Style.HeaderFontStyle)
		size := col.Style.HeaderFontSize
		if size <= 0 {
			size = int(d.headerSize)
		}
		fg := colorOrDefault(col.Style.HeaderFontColor, [3]int{0, 0, 0})
		bg := col.Style.HeaderCellColor
		if bg == "" {
			bg = pdfBusinessHeaderFill
		}
		d.drawCell(ci, y, hh, col.Header, fpdfStyle(style), float64(size),
			fg, bg, "C", "header")
	}
	d.pdf.SetY(y + hh + 1.5)
}

// rowHeight computes the rendered height of a row by wrapping every cell and
// taking the tallest cell, so adjacent rows never overlap.
func (d *pdfDoc) rowHeight(cells []models.Cell) float64 {
	maxH := 0.0
	for ci, cell := range cells {
		col := d.columns[ci]
		style, size := cellFont(col, cell)
		lines := d.wrapLines(visualText(cell.String(), d.report.TextDirection), d.colWidths[ci]-2*pdfCellPadH, fpdfStyle(style), size)
		h := float64(len(lines))*d.lineHeight(size) + 2*pdfCellPadV
		if h > maxH {
			maxH = h
		}
	}
	if maxH < pdfMinRowH {
		maxH = pdfMinRowH
	}
	return maxH
}

// drawRow draws one data row. Every cell is drawn at a fixed (x, y) from the
// shared grid with the row height computed by rowHeight.
func (d *pdfDoc) drawRow(cells []models.Cell, height float64, alt bool) {
	y := d.pdf.GetY()
	for ci, cell := range cells {
		col := d.columns[ci]
		style, size := cellFont(col, cell)

		bg := col.Style.ColumnCellColor
		if cell.CellColor != "" {
			bg = cell.CellColor
		} else if alt {
			bg = "#F7F7F7"
		}
		fg := colorOrDefault(col.Style.DataFontColor, [3]int{0x33, 0x33, 0x33})
		align := "L"
		if d.report.TextDirection == models.TextDirectionRTL {
			align = "R"
		}
		switch cell.Type {
		case models.DataTypeNumber:
			align = "R"
		case models.DataTypeBoolean:
			align = "C"
		}
		d.drawCell(ci, y, height, cell.String(), fpdfStyle(style), size, fg, bg, align, "cell")
	}
	d.pdf.SetY(y + height)
}

// drawCell draws one table cell: an optional filled, always-bordered rectangle
// with word-wrapped, vertically centered text. It always starts at (x, y) so
// cells in a row share the exact same Y; position is restored per cell.
func (d *pdfDoc) drawCell(ci int, y, h float64, text, style string, size float64,
	fg [3]int, bg, align, kind string) {

	text = visualText(text, d.report.TextDirection)
	x := d.colStarts[ci]
	w := d.colWidths[ci]

	// Background fill. Data rows deliberately have no grid borders; the header
	// and section rules provide visual structure without a spreadsheet look.
	if bg != "" {
		r, g, b := hexToRGB(bg)
		d.pdf.SetFillColor(r, g, b)
		d.pdf.Rect(x, y, w, h, "F")
	}
	if kind == "header" {
		d.pdf.SetDrawColor(0xD5, 0xE6, 0xFD)
		d.pdf.SetLineWidth(0.2)
		d.pdf.Rect(x, y, w, h, "D")
	}

	// Text: measure with the cell font, wrap to the usable width, then draw
	// line by line with explicit coordinates (never via ln=2 chaining).
	d.pdf.SetFont(fontFamily, style, size)
	d.pdf.SetTextColor(fg[0], fg[1], fg[2])
	lines := d.wrapLines(text, w-2*pdfCellPadH, style, size)
	lh := d.lineHeight(size)
	used := float64(len(lines))*lh + 2*pdfCellPadV
	ty := y + (h-used)/2 // vertical centering within the row
	if ty < y {
		ty = y
	}
	for _, line := range lines {
		d.pdf.SetY(ty)
		d.pdf.SetX(x + pdfCellPadH)
		d.pdf.CellFormat(w-2*pdfCellPadH, lh, line, "", 0, align, false, 0, "")
		d.pdf.Ln(lh)
		ty += lh
	}
	d.emit(kind, x, y, x+w, y+h)
}

// wrapLines wraps text to a maximum width using actual font metrics and
// UTF-8 rune measurement. Explicit newlines are preserved. It sets the current
// font to measure with.
func (d *pdfDoc) wrapLines(text string, width float64, style string, size float64) []string {
	d.pdf.SetFont(fontFamily, style, size)

	var out []string
	for _, seg := range strings.Split(text, "\n") {
		if seg == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(seg)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}

		var cur strings.Builder
		hasCur := false
		for _, w := range words {
			candidate := w
			if hasCur {
				candidate = cur.String() + " " + w
			}
			if d.pdf.GetStringWidth(candidate) <= width {
				cur.Reset()
				cur.WriteString(candidate)
				hasCur = true
				continue
			}
			if !hasCur {
				// Word wider than the column: hard-break on rune boundaries.
				chunks := d.breakWord(w, width)
				out = append(out, chunks[:len(chunks)-1]...)
				cur.Reset()
				cur.WriteString(chunks[len(chunks)-1])
				hasCur = true
				continue
			}
			out = append(out, cur.String())
			cur.Reset()
			cur.WriteString(w)
			hasCur = true
		}
		if hasCur {
			out = append(out, cur.String())
		}
	}
	return out
}

// breakWord splits a single word into chunks that each fit the width, never
// splitting a UTF-8 rune. Returns at least one chunk.
func (d *pdfDoc) breakWord(word string, width float64) []string {
	var chunks []string
	var cur strings.Builder
	for _, r := range word {
		if cur.Len() > 0 && d.pdf.GetStringWidth(cur.String()+string(r)) > width {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	if len(chunks) == 0 {
		chunks = append(chunks, word)
	}
	return chunks
}

func (d *pdfDoc) drawPageFooter() {
	d.pdf.SetY(-pdfFooterZone)
	d.pdf.SetFont(fontFamily, "", 9)
	d.pdf.SetTextColor(0x66, 0x66, 0x66)
	third := d.usableW / 3
	d.pdf.SetX(pdfMarginLeft)
	d.pdf.CellFormat(third, 6, d.report.Footer.Left, "", 0, "L", false, 0, "")
	d.pdf.CellFormat(third, 6, fmt.Sprintf("Page %d", d.pdf.PageNo()), "", 0, "C", false, 0, "")
	d.pdf.CellFormat(third, 6, d.report.Footer.Right, "", 0, "R", false, 0, "")
	d.emit("footer", pdfMarginLeft, d.pageH-pdfFooterZone, pdfMarginLeft+d.usableW, d.pageH-pdfFooterZone+6)
}

func (d *pdfDoc) emit(kind string, x0, y0, x1, y1 float64) {
	if d.hook != nil {
		d.hook(kind, x0, y0, x1, y1)
	}
}

// cellFont resolves the effective font style and size for a cell, falling
// back to column-level directives and library defaults.
func cellFont(col models.Column, cell models.Cell) (string, float64) {
	style := string(cell.FontStyle)
	size := cell.FontSize
	if style == "" || style == "normal" {
		style = string(col.Style.DataFontStyle)
	}
	if size <= 0 {
		size = col.Style.DataFontSize
	}
	if size <= 0 {
		size = defaultDataFontSize
	}
	return fpdfStyle(style), float64(size)
}

func fpdfStyle(s string) string {
	switch s {
	case string(models.FontBold):
		return "B"
	case string(models.FontItalic):
		return "I"
	case string(models.FontUnderline):
		return "U"
	default:
		return ""
	}
}

// hexToRGB parses #RGB or #RRGGBB into RGB components.
func hexToRGB(color string) (int, int, int) {
	s := strings.TrimPrefix(color, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return int(r), int(g), int(b)
}

func colorOrDefault(hex string, fallback [3]int) [3]int {
	if hex == "" {
		return fallback
	}
	r, g, b := hexToRGB(hex)
	return [3]int{r, g, b}
}
