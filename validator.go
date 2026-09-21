package reports

import (
	"regexp"

	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/models"
)

// Supported range for font sizes across all renderers.
const (
	MinFontSize = 6
	MaxFontSize = 100
)

var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// IsValidHexColor reports whether s is a #RGB or #RRGGBB color.
func IsValidHexColor(s string) bool {
	return hexColorRe.MatchString(s)
}

// Validate performs full semantic validation of an internal report model.
// It is called automatically by the Export APIs; call it explicitly when
// building models programmatically and deferring export.
func Validate(report *models.Report) error {
	if report == nil {
		return errors.NewValidationError(errors.ErrNilReport, "", "report is nil")
	}

	if len(report.Columns) == 0 {
		return errors.NewValidationError(errors.ErrEmptyReport, "", "report has no columns")
	}
	if report.TextDirection == "" {
		// Empty is the programmatic equivalent of omitted JSON and means LTR.
	} else if !report.TextDirection.Valid() {
		return errors.NewValidationError(errors.ErrInvalidSchema, "text_direction", "expected LTR or RTL")
	}

	// Column consistency: every column must carry the same number of data rows.
	rowCount := -1
	for ci, col := range report.Columns {
		n := len(col.ColumnData)
		if rowCount == -1 {
			rowCount = n
		} else if n != rowCount {
			return errors.NewValidationError(
				errors.ErrColumnMismatch,
				field(ci),
				"all columns must have the same number of data rows",
			)
		}
	}
	if rowCount == 0 {
		return errors.NewValidationError(errors.ErrEmptyReport, "", "report has no data rows")
	}

	for ci, col := range report.Columns {
		if col.Header == "" {
			return errors.NewValidationError(
				errors.ErrInvalidSchema, field(ci), "column_header is required")
		}
		if err := validateColumnStyle(col.Style, ci); err != nil {
			return err
		}
		for ri, cell := range col.ColumnData {
			if err := validateCell(cell, ci, ri); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateColumnStyle(s models.ColumnStyle, ci int) error {
	if err := validateFontDirective(s.HeaderFontStyle, field(ci), "style.header_font_style"); err != nil {
		return err
	}
	if err := validateFontDirective(s.DataFontStyle, field(ci), "style.data_font_style"); err != nil {
		return err
	}
	if err := validateFontSize(s.HeaderFontSize, field(ci), "style.header_font_size"); err != nil {
		return err
	}
	if err := validateFontSize(s.DataFontSize, field(ci), "style.data_font_size"); err != nil {
		return err
	}
	if err := validateColor(s.HeaderFontColor, field(ci), "style.header_font_color"); err != nil {
		return err
	}
	if err := validateColor(s.HeaderCellColor, field(ci), "style.header_cell_color"); err != nil {
		return err
	}
	if err := validateColor(s.DataFontColor, field(ci), "style.data_font_color"); err != nil {
		return err
	}
	if err := validateColor(s.ColumnCellColor, field(ci), "style.column_cell_color"); err != nil {
		return err
	}
	return nil
}

func validateCell(c models.Cell, ci, ri int) error {
	loc := field(ci, "column_data", ri)

	if err := validateFontDirective(c.FontStyle, loc, "font_style"); err != nil {
		return err
	}
	if err := validateFontSize(c.FontSize, loc, "font_size"); err != nil {
		return err
	}
	if err := validateColor(c.CellColor, loc, "cell_color"); err != nil {
		return err
	}
	if err := validateDataType(c.Type, loc); err != nil {
		return err
	}
	return nil
}

func validateFontDirective(s models.FontStyle, loc, sub string) error {
	if s == "" {
		return nil // unset: renderer default applies
	}
	if !s.Valid() {
		return errors.NewValidationError(
			errors.ErrInvalidFont, loc,
			"invalid font style %q; expected one of normal, bold, italic, underline", s,
		)
	}
	return nil
}

func validateFontSize(size int, loc, sub string) error {
	if size == 0 {
		return nil // unset: renderer default applies
	}
	if size < MinFontSize || size > MaxFontSize {
		return errors.NewValidationError(
			errors.ErrInvalidFontSize, loc,
			"font size %d outside supported range [%d..%d]", size, MinFontSize, MaxFontSize,
		)
	}
	return nil
}

func validateColor(color, loc, sub string) error {
	if color == "" {
		return nil // unset: renderer default applies
	}
	if !IsValidHexColor(color) {
		return errors.NewValidationError(
			errors.ErrInvalidColor, loc,
			"invalid hex color %q; expected #RGB or #RRGGBB", color,
		)
	}
	return nil
}

func validateDataType(t models.DataType, loc string) error {
	if t == "" {
		return errors.NewValidationError(
			errors.ErrInvalidType, loc, "cell type is required (string, number, date, boolean)")
	}
	switch t {
	case models.DataTypeString, models.DataTypeNumber,
		models.DataTypeDate, models.DataTypeBoolean:
		return nil
	default:
		return errors.NewValidationError(
			errors.ErrInvalidType, loc, "unsupported cell type %q", t)
	}
}

// validateOptions checks export option values that affect renderers.
// Unknown formats are intentionally allowed here: the renderer registry is
// the extension point for custom formats (see RegisterRenderer). Whether a
// format has a renderer is decided by the factory lookup.
func validateOptions(opts models.ExportOptions) error {
	if opts.PageOrientation != "" &&
		opts.PageOrientation != models.OrientationPortrait &&
		opts.PageOrientation != models.OrientationLandscape {
		return errors.NewValidationError(
			errors.ErrInvalidSchema, "", "unsupported page orientation %q", opts.PageOrientation)
	}
	return nil
}
