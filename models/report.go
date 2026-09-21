// Package models defines the canonical, format-agnostic domain model for
// reports. Renderers consume these types; parsers produce them. The model is
// intentionally free of any output-format concerns (Excel, PDF, CSV, Word).
package models

import (
	"fmt"
	"math"
)

// DataType is the logical type of a cell value.
type DataType string

// Supported cell data types.
const (
	DataTypeString  DataType = "string"
	DataTypeNumber  DataType = "number"
	DataTypeDate    DataType = "date"
	DataTypeBoolean DataType = "boolean"
)

// FontStyle is a font decoration applied to text.
type FontStyle string

// TextDirection controls render-time layout direction.
type TextDirection string

const (
	TextDirectionLTR TextDirection = "LTR"
	TextDirectionRTL TextDirection = "RTL"
)

func (d TextDirection) Valid() bool {
	return d == TextDirectionLTR || d == TextDirectionRTL
}

// Supported font styles.
const (
	FontNormal    FontStyle = "normal"
	FontBold      FontStyle = "bold"
	FontItalic    FontStyle = "italic"
	FontUnderline FontStyle = "underline"
)

// Valid returns whether the font style is one of the supported values.
func (s FontStyle) Valid() bool {
	switch s {
	case FontNormal, FontBold, FontItalic, FontUnderline:
		return true
	default:
		return false
	}
}

// Format identifies a supported output format.
type Format string

// Supported output formats.
const (
	FormatExcel Format = "excel"
	FormatCSV   Format = "csv"
	FormatPDF   Format = "pdf"
	FormatWord  Format = "word"
)

// String implements fmt.Stringer.
func (f Format) String() string { return string(f) }

// Valid returns whether the format is one of the supported values.
func (f Format) Valid() bool {
	switch f {
	case FormatExcel, FormatCSV, FormatPDF, FormatWord:
		return true
	default:
		return false
	}
}

// Page orientations used by PDF and Word renderers.
const (
	OrientationPortrait  = "portrait"
	OrientationLandscape = "landscape"
)

// Cell is a single data cell. Value is kept as an any so the internal model
// stays independent of any output format; Type records the logical type.
type Cell struct {
	Value     any
	Type      DataType
	FontStyle FontStyle
	FontSize  int
	CellColor string
}

// String renders the cell value as a plain string. Formatting is best-effort:
// numbers are printed without trailing zeros, booleans as true/false, and any
// other value is rendered via fmt.Sprint.
func (c Cell) String() string {
	if c.Value == nil {
		return ""
	}
	switch v := c.Value.(type) {
	case string:
		return v
	case float64:
		return formatNumber(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int64:
		return formatNumber((float64(v)))
	default:
		return fmt.Sprint(v)
	}
}

// ColumnStyle describes styling that applies to an entire column.
type ColumnStyle struct {
	HeaderFontStyle FontStyle
	DataFontStyle   FontStyle
	HeaderFontSize  int
	DataFontSize    int
	HeaderFontColor string
	HeaderCellColor string
	DataFontColor   string
	ColumnCellColor string
}

// Column is one column of a report: its header, its column-level style, and
// its cells. The schema is column-major; renderers transpose to rows.
type Column struct {
	Header     string
	Style      ColumnStyle
	ColumnData []Cell
}

// Header carries the page-level header text.
type Header struct {
	Left  string
	Right string
}

// Footer carries the page-level footer text.
type Footer struct {
	Left  string
	Right string
}

// Report is the canonical internal model, independent of any output format.
type Report struct {
	Title         string
	TextDirection TextDirection
	// Logo holds PNG or JPEG bytes. It is optional and is rendered only by
	// document formats that support images.
	Logo    []byte
	Header  Header
	Footer  Footer
	Columns []Column
}

// RowCount returns the number of data rows in the report, derived from the
// first non-empty column. It returns 0 for a report with no data.
func (r *Report) RowCount() int {
	if r == nil {
		return 0
	}
	for i := range r.Columns {
		if n := len(r.Columns[i].ColumnData); n > 0 {
			return n
		}
	}
	return 0
}

// ColumnCount returns the number of columns.
func (r *Report) ColumnCount() int {
	if r == nil {
		return 0
	}
	return len(r.Columns)
}

func formatNumber(v float64) string {
	switch {
	case math.Abs(v) >= 1_000_000_000:
		return fmt.Sprintf("%.3fB", v/1_000_000_000)
	case math.Abs(v) >= 1_000_000:
		return fmt.Sprintf("%.3fM", v/1_000_000)
	case math.Abs(v) >= 1_000:
		return fmt.Sprintf("%.3fK", v/1_000)
	default:
		return fmt.Sprintf("%.3f", v)
	}
}
