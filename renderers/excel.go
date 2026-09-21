package renderers

import (
	"io"
	"math"
	"strings"

	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/models"
	"github.com/xuri/excelize/v2"
)

// Default font sizes/colors applied when the schema omits them.
const (
	defaultHeaderFontSize = 12
	defaultDataFontSize   = 10
	defaultHeaderColor    = "#000000"
	defaultDataColor      = "#333333"
	defaultHeaderFill     = "#EEEEEE"
)

// ExcelRenderer renders reports into .xlsx using a streaming writer so very
// large datasets never materialize as a full row matrix. It supports styled
// headers, typed cells, auto column widths and frozen header panes.
type ExcelRenderer struct {
	opts models.ExportOptions
}

// NewExcelRenderer builds an ExcelRenderer with the given export options.
func NewExcelRenderer(opts models.ExportOptions) *ExcelRenderer {
	return &ExcelRenderer{opts: opts}
}

// Render implements interfaces.Renderer.
func (r *ExcelRenderer) Render(report *models.Report) ([]byte, error) {
	return r.renderTo(report, nil)
}

// RenderToWriter implements interfaces.WriterRenderer.
func (r *ExcelRenderer) RenderToWriter(report *models.Report, w io.Writer) error {
	_, err := r.renderTo(report, w)
	return err
}

func (r *ExcelRenderer) renderTo(report *models.Report, w io.Writer) ([]byte, error) {
	if err := checkConsistent(report); err != nil {
		return nil, errors.WrapRenderError(string(models.FormatExcel), err)
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := sheetName(r.opts.SheetName)
	if sheet != "Sheet1" {
		if err := f.SetSheetName("Sheet1", sheet); err != nil {
			return nil, renderErr(err)
		}
	}

	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return nil, renderErr(err)
	}

	if r.opts.FreezeHeader {
		if err := sw.SetPanes(&excelize.Panes{
			Freeze:      true,
			YSplit:      1,
			TopLeftCell: "A2",
			ActivePane:  "bottomLeft",
		}); err != nil {
			return nil, renderErr(err)
		}
	}

	styleCache := newStyleCache(f)

	// Column widths must be set before rows are written, so auto-sizing does a
	// cheap pre-scan of the source model (no extra allocations).
	if r.opts.AutoSizeColumns {
		widths := r.computeColumnWidths(report)
		for ci, width := range widths {
			if err := sw.SetColWidth(ci+1, ci+1, width); err != nil {
				return nil, renderErr(err)
			}
		}
	}

	// Header row.
	headers := make([]any, len(report.Columns))
	for ci, col := range report.Columns {
		styleID, err := styleCache.header(col.Style)
		if err != nil {
			return nil, renderErr(err)
		}
		headers[ci] = excelize.Cell{Value: col.Header, StyleID: styleID}
	}
	if err := sw.SetRow("A1", headers); err != nil {
		return nil, renderErr(err)
	}

	// Data rows, streamed.
	it := NewRowIterator(report)
	row := make([]any, len(report.Columns))
	rowNum := 2
	for it.Next() {
		cells := it.Row()
		for ci, cell := range cells {
			styleID, err := styleCache.data(cell, report.Columns[ci].Style)
			if err != nil {
				return nil, renderErr(err)
			}
			row[ci] = excelize.Cell{Value: excelValue(cell), StyleID: styleID}
		}
		cellName, err := excelize.CoordinatesToCellName(1, rowNum)
		if err != nil {
			return nil, renderErr(err)
		}
		if err := sw.SetRow(cellName, row); err != nil {
			return nil, renderErr(err)
		}
		rowNum++
	}

	if err := sw.Flush(); err != nil {
		return nil, renderErr(err)
	}

	if w != nil {
		if _, err := f.WriteTo(w); err != nil {
			return nil, renderErr(err)
		}
		return nil, nil
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, renderErr(err)
	}
	return buf.Bytes(), nil
}

// computeColumnWidths scans the report and estimates the widest cell per
// column, used for auto column sizing in Excel.
func (r *ExcelRenderer) computeColumnWidths(report *models.Report) []float64 {
	widths := make([]float64, len(report.Columns))
	for ci, col := range report.Columns {
		widths[ci] = displayWidth(col.Header, col.Style.HeaderFontSize)
	}
	it := NewRowIterator(report)
	for it.Next() {
		cells := it.Row()
		for ci := range cells {
			size := cells[ci].FontSize
			if size <= 0 {
				size = report.Columns[ci].Style.DataFontSize
			}
			if w := displayWidth(cells[ci].String(), size); w > widths[ci] {
				widths[ci] = w
			}
		}
	}
	return widths
}

func sheetName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Sheet1"
	}
	return name
}

// excelValue converts an internal cell to a value excelize understands,
// honoring the declared type so numbers are numeric cells, not text.
func excelValue(c models.Cell) any {
	if c.Value == nil {
		return ""
	}
	switch c.Type {
	case models.DataTypeNumber:
		return c.Value
	case models.DataTypeBoolean:
		return c.Value
	default:
		return c.String()
	}
}

// displayWidth estimates a column width (in character units) from the longest
// visible string. The estimate is a heuristic; renderers may cap it.
func displayWidth(s string, fontSize int) float64 {
	if fontSize <= 0 {
		fontSize = defaultDataFontSize
	}
	// CJK/wide characters occupy roughly double width.
	runes := []rune(s)
	var units float64
	for _, r := range runes {
		if r > 0x2E80 {
			units += 2
		} else {
			units++
		}
	}
	// Scale by font size relative to the 11pt default and add padding.
	w := units*(float64(fontSize)/11.0) + 2
	return math.Min(50, math.Max(8, w))
}

func renderErr(err error) error {
	return errors.WrapRenderError(string(models.FormatExcel),
		errors.WrapFormatError(string(models.FormatExcel), err))
}

// styleCache memoizes excelize style IDs keyed by resolved visual properties,
// keeping the stylesheet small even for very large reports.
type styleCache struct {
	f   *excelize.File
	ids map[styleKey]int
}

type styleKey struct {
	style     string
	size      float64
	color     string
	bg        string
	bold      bool
	italic    bool
	underline bool
	header    bool
}

func newStyleCache(f *excelize.File) *styleCache {
	return &styleCache{f: f, ids: make(map[styleKey]int)}
}

func (sc *styleCache) header(s models.ColumnStyle) (int, error) {
	key := styleKey{
		style:     string(s.HeaderFontStyle),
		size:      fontPx(s.HeaderFontSize, defaultHeaderFontSize),
		color:     hexOrDefault(s.HeaderFontColor, defaultHeaderColor),
		bg:        defaultHeaderFill,
		bold:      isBold(s.HeaderFontStyle),
		italic:    isItalic(s.HeaderFontStyle),
		underline: isUnderline(s.HeaderFontStyle),
		header:    true,
	}
	return sc.get(key)
}

func (sc *styleCache) data(c models.Cell, col models.ColumnStyle) (int, error) {
	size := c.FontSize
	if size <= 0 {
		size = col.DataFontSize
	}
	style := c.FontStyle
	if style == models.FontNormal {
		style = col.DataFontStyle
	}
	bg := c.CellColor
	if bg == "" {
		bg = col.ColumnCellColor
	}
	key := styleKey{
		style:     string(style),
		size:      fontPx(size, defaultDataFontSize),
		color:     hexOrDefault(col.DataFontColor, defaultDataColor),
		bg:        hexOrDefault(bg, "#FFFFFF"),
		bold:      isBold(style),
		italic:    isItalic(style),
		underline: isUnderline(style),
	}
	return sc.get(key)
}

func (sc *styleCache) get(key styleKey) (int, error) {
	if id, ok := sc.ids[key]; ok {
		return id, nil
	}
	underline := ""
	if key.underline {
		underline = "single"
	}
	fill := ""
	if key.bg != "" && key.bg != "#FFFFFF" {
		fill = key.bg
	}
	style := &excelize.Style{
		Font: &excelize.Font{
			Bold:      key.bold,
			Italic:    key.italic,
			Underline: underline,
			Size:      key.size,
			Color:     hexValue(key.color),
		},
	}
	if fill != "" {
		style.Fill = excelize.Fill{
			Type:    "pattern",
			Pattern: 1,
			Color:   []string{hexValue(fill)},
		}
	}
	id, err := sc.f.NewStyle(style)
	if err != nil {
		return 0, err
	}
	sc.ids[key] = id
	return id, nil
}

func fontPx(size, def int) float64 {
	if size <= 0 {
		return float64(def)
	}
	return float64(size)
}

func hexOrDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// hexValue strips the leading '#' excelize does not expect in color values.
func hexValue(v string) string {
	return strings.TrimPrefix(v, "#")
}

func isBold(s models.FontStyle) bool {
	return s == models.FontBold
}

func isItalic(s models.FontStyle) bool {
	return s == models.FontItalic
}

func isUnderline(s models.FontStyle) bool {
	return s == models.FontUnderline
}
