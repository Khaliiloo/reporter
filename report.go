// Package reports is a reusable, framework-agnostic report generation
// library. It accepts a JSON report definition (or a programmatically built
// model) and exports it to Excel, CSV, PDF or Word, returning bytes or
// streaming to an io.Writer so consumers can save to disk, upload to object
// storage, stream over HTTP, attach to emails, or persist in a database.
//
// Typical usage:
//
//	report, err := reports.Parse(jsonData)
//	if err != nil { return err }
//
//	file, err := reports.Export(report, reports.FormatExcel)
//	if err != nil { return err }
//
// The package is composed of five separable phases: parsing, validation,
// internal modeling, rendering and the export API. Renderers are pluggable:
// register new formats via reports.RegisterRenderer without touching existing
// code.
package reports

import (
	"io"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/errors"
	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/interfaces"
	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

// Renderer is re-exported so consumers can use either reports.Renderer or
// interfaces.Renderer.
type Renderer = interfaces.Renderer

// WriterRenderer is re-exported from the interfaces package.
type WriterRenderer = interfaces.WriterRenderer

// Sentinel errors are re-exported from the errors package so consumers can
// write errors.Is(err, reports.ErrInvalidColor) without importing internals.
var (
	ErrParseFailed       = errors.ErrParseFailed
	ErrInvalidSchema     = errors.ErrInvalidSchema
	ErrInvalidColor      = errors.ErrInvalidColor
	ErrInvalidFont       = errors.ErrInvalidFont
	ErrInvalidType       = errors.ErrInvalidType
	ErrInvalidFontSize   = errors.ErrInvalidFontSize
	ErrEmptyReport       = errors.ErrEmptyReport
	ErrColumnMismatch    = errors.ErrColumnMismatch
	ErrNilReport         = errors.ErrNilReport
	ErrUnsupportedFormat = errors.ErrUnsupportedFormat
	ErrRenderFailed      = errors.ErrRenderFailed
)

// RenderError is a structured error wrapping a renderer failure.
type RenderError = errors.RenderError

// FormatError is a structured error wrapping a format-specific failure.
type FormatError = errors.FormatError

// ValidationError is a structured error carrying the offending field.
type ValidationError = errors.ValidationError

// Export writes the rendered report to w using options. It validates the
// report, resolves the renderer from the factory, and streams the output.
// The writer is not closed.
func ExportToWriter(report *models.Report, format models.Format, w io.Writer) error {
	return ExportToWriterWithOptions(report, defaultOptionsFor(format), w)
}

// ExportToWriterWithOptions is ExportToWriter with full export options.
func ExportToWriterWithOptions(report *models.Report, opts models.ExportOptions, w io.Writer) error {
	if err := Validate(report); err != nil {
		return err
	}
	r, err := NewRendererWithOptions(opts.Format, opts)
	if err != nil {
		return err
	}
	if wr, ok := r.(interfaces.WriterRenderer); ok {
		return wr.RenderToWriter(report, w)
	}
	b, err := r.Render(report)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// models re-exports
type (
	// Report is the canonical internal model.
	Report = models.Report
	// Column is one column of a report.
	Column = models.Column
	// ColumnStyle is column-level styling.
	ColumnStyle = models.ColumnStyle
	// Cell is a single data cell.
	Cell = models.Cell
	// Header is the page-level header.
	Header = models.Header
	// Footer is the page-level footer.
	Footer = models.Footer
	// ExportOptions configures an export.
	ExportOptions = models.ExportOptions
	// Format identifies an output format.
	Format = models.Format
	// DataType identifies a cell value type.
	DataType = models.DataType
	// FontStyle identifies a font decoration.
	FontStyle = models.FontStyle
	// TextDirection controls render-time LTR/RTL layout.
	TextDirection = models.TextDirection
)

// Format and style constants re-exported from models.
const (
	FormatExcel = models.FormatExcel
	FormatCSV   = models.FormatCSV
	FormatPDF   = models.FormatPDF
	FormatWord  = models.FormatWord

	OrientationPortrait  = models.OrientationPortrait
	OrientationLandscape = models.OrientationLandscape

	DataTypeString  = models.DataTypeString
	DataTypeNumber  = models.DataTypeNumber
	DataTypeDate    = models.DataTypeDate
	DataTypeBoolean = models.DataTypeBoolean

	FontNormal    = models.FontNormal
	FontBold      = models.FontBold
	FontItalic    = models.FontItalic
	FontUnderline = models.FontUnderline

	TextDirectionLTR = models.TextDirectionLTR
	TextDirectionRTL = models.TextDirectionRTL
)
