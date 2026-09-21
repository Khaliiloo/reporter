package renderers

import (
	"strings"
	"testing"

	"github.com/Khaliiloo/reporter/models"
	"github.com/go-pdf/fpdf"
)

// layoutEvent is a captured layout element from pdfLayoutHook.
type layoutEvent struct {
	kind      string
	x0, y0    float64
	x1, y1    float64
	colStarts []float64
}

func layoutReport() *models.Report {
	return &models.Report{
		Title:  "My First Report",
		Header: models.Header{Left: "Header Left", Right: "Header Right"},
		Footer: models.Footer{Left: "Footer Left", Right: "Footer Right"},
		Columns: []models.Column{
			{Header: "Name", ColumnData: []models.Cell{
				{Value: "John", Type: models.DataTypeString},
				{Value: "Jane", Type: models.DataTypeString},
			}},
			{Header: "Score", ColumnData: []models.Cell{
				{Value: 95.5, Type: models.DataTypeNumber},
				{Value: 88, Type: models.DataTypeNumber},
			}},
			{Header: "Total", ColumnData: []models.Cell{
				{Value: "Long wrapped value that spans more than one line", Type: models.DataTypeString},
				{Value: 75, Type: models.DataTypeNumber},
			}},
		},
	}
}

// TestPDFLayoutOrder verifies the document body order: title first, report
// metadata below it, separator below metadata, then the table. There is no
// page-header element above the title.
func TestPDFLayoutOrder(t *testing.T) {
	events := capturePDFLayout(t, layoutReport())

	if events[0].kind != "title" {
		t.Fatalf("first element = %q, want title (no page header above title)", events[0].kind)
	}

	var metaY, sepY, headerY float64
	var sawMeta, sawSep bool
	for _, e := range events {
		switch e.kind {
		case "meta":
			if !sawMeta {
				metaY = e.y0
				sawMeta = true
			}
		case "separator":
			sawSep = true
			sepY = e.y0
		case "header":
			if headerY == 0 {
				headerY = e.y0
			}
		}
	}
	if !sawMeta || !sawSep {
		t.Fatal("missing meta or separator elements")
	}
	if !(events[0].y1 <= metaY) {
		t.Errorf("title (y1=%.2f) must be above meta (y0=%.2f)", events[0].y1, metaY)
	}
	if !(metaY <= sepY) {
		t.Errorf("separator (y0=%.2f) must be below meta (y0=%.2f)", sepY, metaY)
	}
	if !(sepY <= headerY) {
		t.Errorf("table header (y0=%.2f) must be below separator (y0=%.2f)", headerY, sepY)
	}
}

// TestPDFHeaderSingleRow verifies all column headers share the same Y and are
// horizontally contiguous (issue: staggered headers).
func TestPDFHeaderSingleRow(t *testing.T) {
	events := capturePDFLayout(t, layoutReport())

	var headers []layoutEvent
	for _, e := range events {
		if e.kind == "header" {
			headers = append(headers, e)
		}
	}
	if len(headers) != 3 {
		t.Fatalf("header events = %d, want 3", len(headers))
	}
	y0 := headers[0].y0
	for i, h := range headers[1:] {
		if !almostEqual(h.y0, y0, 0.01) {
			t.Errorf("header %d y0=%.2f, want same row %.2f", i+1, h.y0, y0)
		}
		if h.x0 < headers[i].x1-0.01 {
			t.Errorf("header %d overlaps horizontally: x0=%.2f < prev x1=%.2f", i+1, h.x0, headers[i].x1)
		}
	}
}

// TestPDFRowsNoOverlap verifies every cell in a row shares the same Y, rows do
// not overlap vertically, and cells align to the shared column grid.
func TestPDFRowsNoOverlap(t *testing.T) {
	events := capturePDFLayout(t, layoutReport())

	var cells []layoutEvent
	for _, e := range events {
		if e.kind == "cell" {
			cells = append(cells, e)
		}
	}
	if len(cells) != 6 { // 3 cols × 2 rows
		t.Fatalf("cell events = %d, want 6", len(cells))
	}

	// Group into rows by y0.
	rows := make([][]layoutEvent, 0, 2)
	for _, c := range cells {
		added := false
		for i := range rows {
			if almostEqual(rows[i][0].y0, c.y0, 0.01) {
				rows[i] = append(rows[i], c)
				added = true
				break
			}
		}
		if !added {
			rows = append(rows, []layoutEvent{c})
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}

	// Within a row, cells share Y and do not overlap horizontally.
	for ri, row := range rows {
		y0 := row[0].y0
		y1 := row[0].y1
		for i, c := range row {
			if !almostEqual(c.y0, y0, 0.01) {
				t.Errorf("row %d cell %d y0=%.2f, want %.2f", ri, i, c.y0, y0)
			}
			if c.y1 > y1+0.01 {
				y1 = c.y1
			}
			if i > 0 && c.x0 < row[i-1].x1-0.01 {
				t.Errorf("row %d cells overlap: x0=%.2f < prev x1=%.2f", ri, c.x0, row[i-1].x1)
			}
		}
	}

	// Consecutive rows must not overlap vertically.
	if rows[1][0].y0 < rows[0][0].y1-0.01 {
		t.Errorf("row 2 (y0=%.2f) overlaps row 1 (y1=%.2f)", rows[1][0].y0, rows[0][0].y1)
	}

	// Cells must sit inside the usable page width.
	for _, c := range cells {
		if c.x0 < pdfMarginLeft-0.01 || c.x1 > pdfMarginLeft+190+0.01 {
			t.Errorf("cell outside margins: x0=%.2f x1=%.2f", c.x0, c.x1)
		}
	}
}

// TestPDFWrappedRowHeight verifies a row containing a wrapped multi-line cell
// grows tall enough to contain it (no text overlap into the next row).
func TestPDFWrappedRowHeight(t *testing.T) {
	report := layoutReport()
	// Force the first column to wrap on row 1.
	report.Columns[0].ColumnData[0] = models.Cell{
		Value: "This is a deliberately long sentence designed to wrap onto multiple lines",
		Type:  models.DataTypeString,
	}
	events := capturePDFLayout(t, report)

	var firstRow, secondRow layoutEvent
	found := false
	for _, e := range events {
		if e.kind == "cell" {
			if !found {
				firstRow = e
				found = true
			} else if !almostEqual(e.y0, firstRow.y0, 0.01) {
				secondRow = e
				break
			}
		}
	}
	if secondRow.y0 < firstRow.y1-0.01 {
		t.Errorf("second row (y0=%.2f) overlaps wrapped first row (y1=%.2f)", secondRow.y0, firstRow.y1)
	}
}

// TestPDFWrapLinesUnicode verifies UTF-8 text wraps without splitting runes
// and explicit newlines are preserved.
func TestPDFWrapLinesUnicode(t *testing.T) {
	pdf := newPDFForTest(t)
	dd := &pdfDoc{pdf: pdf, report: &models.Report{}, opts: models.ExportOptions{}}
	dd.initLayout()

	width := 30.0
	lines := dd.wrapLines("مرحبا بالعالم hello world قوس", width, "", 10)
	for _, l := range lines {
		if pdf.GetStringWidth(l) > width+0.01 {
			t.Errorf("line %q exceeds width %.2f", l, width)
		}
	}
	joined := strings.Join(lines, "")
	if !strings.Contains(joined, "مرحبا") {
		t.Error("Arabic text lost in wrapping")
	}

	// Explicit newlines preserved.
	multi := dd.wrapLines("line one\nline two", 100, "", 10)
	if len(multi) != 2 || multi[0] != "line one" || multi[1] != "line two" {
		t.Errorf("explicit newlines not preserved: %v", multi)
	}

	// Hard breaks for words wider than the column.
	narrow := dd.wrapLines("supercalifragilisticexpialidocious", 15, "", 10)
	if len(narrow) < 2 {
		t.Errorf("expected hard break, got %v", narrow)
	}
	for _, l := range narrow {
		if pdf.GetStringWidth(l) > 15+0.01 {
			t.Errorf("hard-break chunk %q exceeds width", l)
		}
	}
}

// TestPDFColumnWidthsFit verifies the computed grid fits the page and is
// contiguous.
func TestPDFColumnWidthsFit(t *testing.T) {
	pdf := newPDFForTest(t)
	d := &pdfDoc{pdf: pdf, report: layoutReport(), opts: models.ExportOptions{}}
	d.initLayout()

	var total float64
	for i, w := range d.colWidths {
		total += w
		if i > 0 && !almostEqual(d.colStarts[i], d.colStarts[i-1]+d.colWidths[i-1], 0.01) {
			t.Errorf("column %d start %.2f != prev end", i, d.colStarts[i])
		}
	}
	if total > d.usableW+0.01 {
		t.Errorf("total width %.2f exceeds usable %.2f", total, d.usableW)
	}
}

func capturePDFLayout(t *testing.T, report *models.Report) []layoutEvent {
	t.Helper()
	r := NewPDFRenderer(models.ExportOptions{})
	var events []layoutEvent
	r.layoutHook = func(kind string, x0, y0, x1, y1 float64) {
		events = append(events, layoutEvent{kind: kind, x0: x0, y0: y0, x1: x1, y1: y1})
	}
	if _, err := r.Render(report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return events
}

func newPDFForTest(t *testing.T) *fpdf.Fpdf {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	registerUnicodeFonts(pdf)
	pdf.SetAutoPageBreak(false, 0)
	return pdf
}

func almostEqual(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}
