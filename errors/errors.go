// Package errors defines the typed, inspectable error surface of the reports
// library. It has zero external dependencies so it can be imported by
// consumers without pulling in the rest of the module.
//
// Use errors.Is / errors.As (stdlib) to test for specific failure classes:
//
//	if errors.Is(err, reports.ErrInvalidColor) { ... }
//
// Most errors returned by the library wrap one of these sentinels, so
// structured error handling works across parse, validate and render phases.
package errors

import (
	stderrors "errors"
	"fmt"
)

// Sentinel errors identify the failure class. They are safe to compare with
// errors.Is because every returned error wraps one of them.
var (
	// ErrParseFailed indicates the input could not be parsed into a report.
	ErrParseFailed = stderrors.New("reports: parse failed")

	// ErrInvalidSchema indicates structurally invalid report data.
	ErrInvalidSchema = stderrors.New("reports: invalid schema")

	// ErrInvalidColor indicates a malformed hex color value.
	ErrInvalidColor = stderrors.New("reports: invalid color")

	// ErrInvalidFont indicates an unsupported font style directive.
	ErrInvalidFont = stderrors.New("reports: invalid font style")

	// ErrInvalidType indicates an unsupported cell data type directive.
	ErrInvalidType = stderrors.New("reports: invalid data type")

	// ErrInvalidFontSize indicates a font size outside the supported range.
	ErrInvalidFontSize = stderrors.New("reports: invalid font size")

	// ErrEmptyReport indicates a report with no columns or no data rows.
	ErrEmptyReport = stderrors.New("reports: empty report")

	// ErrColumnMismatch indicates columns with inconsistent data lengths.
	ErrColumnMismatch = stderrors.New("reports: column data length mismatch")

	// ErrNilReport indicates a nil report was passed to the API.
	ErrNilReport = stderrors.New("reports: nil report")

	// ErrUnsupportedFormat indicates a format with no registered renderer.
	ErrUnsupportedFormat = stderrors.New("reports: unsupported format")

	// ErrRenderFailed indicates a renderer failed to produce output.
	ErrRenderFailed = stderrors.New("reports: render failed")
)

// ValidationError is a structured error carrying the offending field and a
// human-readable detail, in addition to the failure class.
type ValidationError struct {
	// Kind is the sentinel error class (e.g. ErrInvalidColor).
	Kind error
	// Field locates the offending input, e.g. "data[2].column_data[1].cell_color".
	Field string
	// Detail is a human-readable explanation.
	Detail string
}

// Error implements error.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%v: %s (%s)", e.Kind, e.Field, e.Detail)
	}
	return fmt.Sprintf("%v: %s", e.Kind, e.Detail)
}

// Unwrap exposes the underlying class so errors.Is works.
func (e *ValidationError) Unwrap() error { return e.Kind }

// FormatError is a structured error indicating a renderer for a specific
// output format failed. It is always returned wrapped by a RenderError.
type FormatError struct {
	Format string
	Err    error
}

// Error implements error.
func (e *FormatError) Error() string {
	return fmt.Sprintf("reports: %s renderer: %v", e.Format, e.Err)
}

// Unwrap exposes the underlying error so errors.Is / errors.As work.
func (e *FormatError) Unwrap() error { return e.Err }

// RenderError is returned when a rendering phase fails. It carries the format
// and a wrapped cause that is itself a *FormatError when the failure is
// format-specific.
type RenderError struct {
	Format string
	Err    error
}

// Error implements error.
func (e *RenderError) Error() string {
	return fmt.Sprintf("reports: failed to render %s: %v", e.Format, e.Err)
}

// Unwrap exposes the underlying error.
func (e *RenderError) Unwrap() error { return e.Err }

// NewValidationError builds a *ValidationError. Optional args are formatted
// into detail using fmt.Sprintf semantics.
func NewValidationError(kind error, field, detail string, args ...any) error {
	if len(args) > 0 {
		detail = fmt.Sprintf(detail, args...)
	}
	return &ValidationError{Kind: kind, Field: field, Detail: detail}
}

// WrapRenderError wraps a renderer failure. The wrapped cause should be a
// *FormatError (or any error) from the concrete renderer.
func WrapRenderError(format string, err error) error {
	return &RenderError{Format: format, Err: err}
}

// WrapFormatError wraps a format-specific renderer failure.
func WrapFormatError(format string, err error) error {
	return &FormatError{Format: format, Err: err}
}
