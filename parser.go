package reports

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/errors"
	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

// Parse parses the JSON report definition into the canonical internal model.
//
// Parsing is structural: it decodes the schema and coerces values into typed
// cells. Semantic validation (colors, font styles, sizes, column consistency)
// is performed by Validate, which the high-level Export API calls
// automatically.
func Parse(data []byte) (*models.Report, error) {
	return ParseReader(bytes.NewReader(data))
}

// ParseReader is Parse for io.Reader input.
func ParseReader(r io.Reader) (*models.Report, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()

	var raw rawReport
	if err := dec.Decode(&raw); err != nil {
		return nil, errors.NewValidationError(
			errors.ErrParseFailed, "", fmt.Sprintf("invalid JSON: %v", err))
	}

	report, err := raw.toModel()
	if err != nil {
		return nil, err
	}
	return report, nil
}

// rawReport mirrors the JSON wire schema.
type rawReport struct {
	Title         string          `json:"title"`
	TextDirection string          `json:"text_direction"`
	Logo          json.RawMessage `json:"logo"`
	HeaderLeft    string          `json:"header_left"`
	HeaderRight   string          `json:"header_right"`
	FooterLeft    string          `json:"footer_left"`
	FooterRight   string          `json:"footer_right"`
	Data          []rawColumn     `json:"data"`
}

// rawColumn mirrors the column object of the JSON schema.
type rawColumn struct {
	ColumnHeader string         `json:"column_header"`
	Style        rawColumnStyle `json:"style"`
	ColumnData   []rawCell      `json:"column_data"`
}

// rawColumnStyle mirrors the column style object.
type rawColumnStyle struct {
	HeaderFontStyle string `json:"header_font_style"`
	DataFontStyle   string `json:"data_font_style"`
	HeaderFontSize  int    `json:"header_font_size"`
	DataFontSize    int    `json:"data_font_size"`
	HeaderFontColor string `json:"header_font_color"`
	HeaderCellColor string `json:"header_cell_color"`
	DataFontColor   string `json:"data_font_color"`
	ColumnCellColor string `json:"column_cell_color"`
}

// rawCell mirrors a single cell object. Value is kept raw so the declared
// type can drive coercion.
type rawCell struct {
	Value     json.RawMessage `json:"value"`
	Type      string          `json:"type"`
	FontStyle string          `json:"font_style"`
	FontSize  int             `json:"font_size"`
	CellColor string          `json:"cell_color"`
}

func (r rawReport) toModel() (*models.Report, error) {
	logo, err := parseLogo(r.Logo)
	if err != nil {
		return nil, err
	}
	report := &models.Report{
		Title:         r.Title,
		TextDirection: normalizeTextDirection(r.TextDirection),
		Logo:          logo,
		Header: models.Header{
			Left:  r.HeaderLeft,
			Right: r.HeaderRight,
		},
		Footer: models.Footer{
			Left:  r.FooterLeft,
			Right: r.FooterRight,
		},
		Columns: make([]models.Column, 0, len(r.Data)),
	}

	for ci, rc := range r.Data {
		col := models.Column{
			Header: rc.ColumnHeader,
			Style: models.ColumnStyle{
				HeaderFontStyle: normalizeFontStyle(rc.Style.HeaderFontStyle),
				DataFontStyle:   normalizeFontStyle(rc.Style.DataFontStyle),
				HeaderFontSize:  rc.Style.HeaderFontSize,
				DataFontSize:    rc.Style.DataFontSize,
				HeaderFontColor: rc.Style.HeaderFontColor,
				HeaderCellColor: rc.Style.HeaderCellColor,
				DataFontColor:   rc.Style.DataFontColor,
				ColumnCellColor: rc.Style.ColumnCellColor,
			},
			ColumnData: make([]models.Cell, 0, len(rc.ColumnData)),
		}

		for ri, rc := range rc.ColumnData {
			cell, err := parseCell(rc, field("data", ci, "column_data", ri))
			if err != nil {
				return nil, err
			}
			col.ColumnData = append(col.ColumnData, cell)
		}
		report.Columns = append(report.Columns, col)
	}

	return report, nil
}

func normalizeTextDirection(s string) models.TextDirection {
	if strings.EqualFold(strings.TrimSpace(s), string(models.TextDirectionRTL)) {
		return models.TextDirectionRTL
	}
	return models.TextDirectionLTR
}

// parseLogo accepts a base64 string (including a data URI) or a JSON byte
// array. JSON has no byte primitive, so base64 is the portable wire form.
func parseLogo(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		if i := strings.Index(encoded, ","); strings.HasPrefix(encoded, "data:") && i >= 0 {
			encoded = encoded[i+1:]
		}
		b, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.NewValidationError(errors.ErrInvalidSchema, "logo", "logo must be base64-encoded PNG or JPEG data")
		}
		return b, nil
	}
	var b []byte
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, errors.NewValidationError(errors.ErrInvalidSchema, "logo", "logo must be base64 text or a byte array")
	}
	return b, nil
}

// parseCell coerces a raw cell into a typed internal cell.
func parseCell(rc rawCell, loc string) (models.Cell, error) {
	cell := models.Cell{
		FontStyle: normalizeFontStyle(rc.FontStyle),
		FontSize:  rc.FontSize,
		CellColor: rc.CellColor,
	}

	declared := normalizeDataType(rc.Type)
	value, typ, err := coerceValue(rc.Value, declared)
	if err != nil {
		return models.Cell{}, errors.NewValidationError(
			errors.ErrInvalidType, loc, err.Error())
	}

	cell.Value = value
	cell.Type = typ
	return cell, nil
}

// coerceValue turns a raw JSON value into a typed value. The declared type
// drives coercion; when absent, the type is inferred from the JSON kind.
func coerceValue(raw json.RawMessage, declared models.DataType) (any, models.DataType, error) {
	if len(raw) == 0 {
		return "", models.DataTypeString, nil
	}

	switch declared {
	case models.DataTypeNumber:
		v, err := toNumber(raw)
		if err != nil {
			return nil, declared, err
		}
		return v, declared, nil

	case models.DataTypeBoolean:
		v, err := toBool(raw)
		if err != nil {
			return nil, declared, err
		}
		return v, declared, nil

	case models.DataTypeDate:
		s, err := toString(raw)
		if err != nil {
			return nil, declared, err
		}
		return s, declared, nil

	case models.DataTypeString:
		s, err := toString(raw)
		if err != nil {
			return nil, declared, err
		}
		return s, declared, nil

	case "":
		// Infer from the JSON kind.
		return inferValue(raw)

	default:
		return nil, declared, fmt.Errorf("unsupported type %q", declared)
	}
}

func inferValue(raw json.RawMessage) (any, models.DataType, error) {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "null":
		return "", models.DataTypeString, nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, models.DataTypeString, err
		}
		return s, models.DataTypeString, nil
	case len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '['):
		return nil, models.DataTypeString,
			fmt.Errorf("object/array value is not a supported cell type")
	default:
		// Try number then boolean.
		if v, err := toNumber(raw); err == nil {
			return v, models.DataTypeNumber, nil
		}
		if v, err := toBool(raw); err == nil {
			return v, models.DataTypeBoolean, nil
		}
		return nil, models.DataTypeString,
			fmt.Errorf("value %s is not a supported cell type", trimmed)
	}
}

func toNumber(raw json.RawMessage) (float64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if f, err := n.Float64(); err == nil {
			return f, nil
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, nil
		}
	}
	return 0, fmt.Errorf("cannot coerce %s to number", strings.TrimSpace(string(raw)))
}

func toBool(raw json.RawMessage) (bool, error) {
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
	}
	return false, fmt.Errorf("cannot coerce %s to boolean", strings.TrimSpace(string(raw)))
}

func toString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	// Accept scalar JSON values by re-serializing them.
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		if v == nil {
			return "", nil
		}
		b, _ := json.Marshal(v)
		return string(b), nil
	}
	return "", fmt.Errorf("cannot coerce %s to string", strings.TrimSpace(string(raw)))
}

func normalizeFontStyle(s string) models.FontStyle {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bold":
		return models.FontBold
	case "italic":
		return models.FontItalic
	case "underline":
		return models.FontUnderline
	case "normal", "":
		return models.FontNormal
	default:
		return models.FontStyle(strings.ToLower(strings.TrimSpace(s)))
	}
}

func normalizeDataType(s string) models.DataType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "string":
		return models.DataTypeString
	case "number":
		return models.DataTypeNumber
	case "date":
		return models.DataTypeDate
	case "boolean":
		return models.DataTypeBoolean
	case "":
		return ""
	default:
		return models.DataType(strings.ToLower(strings.TrimSpace(s)))
	}
}

// field builds a human-readable location string for validation errors.
func field(parts ...any) string {
	var b strings.Builder
	b.WriteString("data")
	for _, p := range parts {
		b.WriteString(fmt.Sprintf("[%v]", p))
	}
	return b.String()
}
