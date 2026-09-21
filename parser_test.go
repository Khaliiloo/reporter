package reports

import (
	"errors"
	"strings"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

const sampleJSON = `{
  "title": "Sales Report",
  "header_left": "Rehlaa Travel",
  "header_right": "Q1 2026",
  "footer_left": "confidential",
  "footer_right": "page",
  "data": [
    {
      "column_header": "Name",
      "style": {
        "header_font_style": "bold",
        "header_font_size": 12,
        "data_font_size": 11
      },
      "column_data": [
        { "value": "John", "type": "string" },
        { "value": "Jane", "type": "string" }
      ]
    },
    {
      "column_header": "Age",
      "style": {
        "data_font_style": "italic",
        "data_font_color": "#555555"
      },
      "column_data": [
        { "value": 25, "type": "number" },
        { "value": 30, "type": "number" }
      ]
    }
  ]
}`

func TestParse(t *testing.T) {
	report, err := Parse([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if report.Title != "Sales Report" {
		t.Errorf("Title = %q", report.Title)
	}
	if report.Header.Left != "Rehlaa Travel" || report.Header.Right != "Q1 2026" {
		t.Errorf("Header = %+v", report.Header)
	}
	if report.Footer.Left != "confidential" || report.Footer.Right != "page" {
		t.Errorf("Footer = %+v", report.Footer)
	}
	if report.ColumnCount() != 2 {
		t.Fatalf("ColumnCount = %d", report.ColumnCount())
	}
	if report.RowCount() != 2 {
		t.Fatalf("RowCount = %d", report.RowCount())
	}

	c0 := report.Columns[0]
	if c0.Header != "Name" {
		t.Errorf("Column[0].Header = %q", c0.Header)
	}
	if c0.Style.HeaderFontStyle != models.FontBold {
		t.Errorf("HeaderFontStyle = %q", c0.Style.HeaderFontStyle)
	}
	if c0.Style.HeaderFontSize != 12 {
		t.Errorf("HeaderFontSize = %d", c0.Style.HeaderFontSize)
	}
	if got := c0.ColumnData[0].String(); got != "John" {
		t.Errorf("cell[0][0] = %q", got)
	}

	c1 := report.Columns[1]
	if got, ok := c1.ColumnData[0].Value.(float64); !ok || got != 25 {
		t.Errorf("cell[1][0] = %v (%T)", c1.ColumnData[0].Value, c1.ColumnData[0].Value)
	}
	if c1.ColumnData[0].Type != models.DataTypeNumber {
		t.Errorf("cell[1][0] type = %q", c1.ColumnData[0].Type)
	}
}

func TestParseInfersTypes(t *testing.T) {
	report, err := Parse([]byte(`{
		"data": [
			{
				"column_header": "Mixed",
				"column_data": [
					{"value": "text"},
					{"value": 42},
					{"value": true},
					{"value": null}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cols := report.Columns[0].ColumnData
	want := []models.DataType{
		models.DataTypeString,
		models.DataTypeNumber,
		models.DataTypeBoolean,
		models.DataTypeString,
	}
	for i, w := range want {
		if cols[i].Type != w {
			t.Errorf("cell[%d] type = %q, want %q", i, cols[i].Type, w)
		}
	}
	if cols[3].String() != "" {
		t.Errorf("null cell string = %q", cols[3].String())
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse([]byte(`{"data": [}`))
	if !errors.Is(err, ErrParseFailed) {
		t.Fatalf("expected ErrParseFailed, got %v", err)
	}
}

func TestParseInvalidCellType(t *testing.T) {
	_, err := Parse([]byte(`{
		"data": [{
			"column_header": "A",
			"column_data": [{"value": 1, "type": "hexadecimal"}]
		}]
	}`))
	if !errors.Is(err, ErrInvalidType) {
		t.Fatalf("expected ErrInvalidType, got %v", err)
	}
}

func TestParseCoercesNumberFromString(t *testing.T) {
	report, err := Parse([]byte(`{
		"data": [{
			"column_header": "A",
			"column_data": [{"value": "3.5", "type": "number"}]
		}]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := report.Columns[0].ColumnData[0].Value; got != 3.5 {
		t.Errorf("value = %v", got)
	}
}

func TestParseReader(t *testing.T) {
	report, err := ParseReader(strings.NewReader(sampleJSON))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	if report.Title != "Sales Report" {
		t.Errorf("Title = %q", report.Title)
	}
}
