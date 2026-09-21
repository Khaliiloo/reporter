package reports

import (
	"errors"
	"strings"
	"testing"

	"github.com/Khaliiloo/reporter/models"
)

func TestValidateValid(t *testing.T) {
	report, err := Parse([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(report); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateNilReport(t *testing.T) {
	if err := Validate(nil); !errors.Is(err, ErrNilReport) {
		t.Fatalf("expected ErrNilReport, got %v", err)
	}
}

func TestValidateEmptyReport(t *testing.T) {
	if err := Validate(&models.Report{}); !errors.Is(err, ErrEmptyReport) {
		t.Fatalf("expected ErrEmptyReport, got %v", err)
	}
}

func TestValidateNoDataRows(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{Header: "A", ColumnData: []models.Cell{}},
		},
	}
	if err := Validate(r); !errors.Is(err, ErrEmptyReport) {
		t.Fatalf("expected ErrEmptyReport, got %v", err)
	}
}

func TestValidateMissingHeader(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{ColumnData: []models.Cell{{Value: "x", Type: models.DataTypeString}}},
		},
	}
	if err := Validate(r); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestValidateColumnMismatch(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{Header: "A", ColumnData: []models.Cell{
				{Value: "1", Type: models.DataTypeString},
				{Value: "2", Type: models.DataTypeString},
			}},
			{Header: "B", ColumnData: []models.Cell{
				{Value: "x", Type: models.DataTypeString},
			}},
		},
	}
	err := Validate(r)
	if !errors.Is(err, ErrColumnMismatch) {
		t.Fatalf("expected ErrColumnMismatch, got %v", err)
	}
	if ve, ok := err.(*ValidationError); ok && !strings.Contains(ve.Field, "data[1]") {
		t.Errorf("unexpected field %q", ve.Field)
	}
}

func TestValidateInvalidColor(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{
				Header:     "A",
				Style:      models.ColumnStyle{HeaderFontColor: "not-a-color"},
				ColumnData: []models.Cell{{Value: "x", Type: models.DataTypeString}},
			},
		},
	}
	err := Validate(r)
	if !errors.Is(err, ErrInvalidColor) {
		t.Fatalf("expected ErrInvalidColor, got %v", err)
	}
	if ve, ok := err.(*ValidationError); ok && ve.Kind != ErrInvalidColor {
		t.Errorf("Kind = %v", ve.Kind)
	}
}

func TestValidateInvalidFontStyle(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{
				Header: "A",
				ColumnData: []models.Cell{
					{Value: "x", Type: models.DataTypeString, FontStyle: "blinking"},
				},
			},
		},
	}
	if err := Validate(r); !errors.Is(err, ErrInvalidFont) {
		t.Fatalf("expected ErrInvalidFont, got %v", err)
	}
}

func TestValidateInvalidFontSize(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{
				Header: "A",
				ColumnData: []models.Cell{
					{Value: "x", Type: models.DataTypeString, FontSize: 4000},
				},
			},
		},
	}
	if err := Validate(r); !errors.Is(err, ErrInvalidFontSize) {
		t.Fatalf("expected ErrInvalidFontSize, got %v", err)
	}
}

func TestValidateUnsupportedDataType(t *testing.T) {
	r := &models.Report{
		Columns: []models.Column{
			{
				Header: "A",
				ColumnData: []models.Cell{
					{Value: "x", Type: "blob"},
				},
			},
		},
	}
	if err := Validate(r); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("expected ErrInvalidType, got %v", err)
	}
}

func TestValidateOptions(t *testing.T) {
	opts := models.ExportOptions{PageOrientation: "diagonal"}
	if err := validateOptions(opts); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
	// Unknown formats pass option validation; the factory lookup rejects them.
	if _, err := NewRendererWithOptions(models.Format("html"), models.ExportOptions{Format: "html"}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestIsValidHexColor(t *testing.T) {
	for _, c := range []string{"#000000", "#FFF", "#abcdef", "#AbCdEf"} {
		if !IsValidHexColor(c) {
			t.Errorf("IsValidHexColor(%q) = false", c)
		}
	}
	for _, c := range []string{"", "000000", "#GGGGGG", "#12345", "red"} {
		if IsValidHexColor(c) {
			t.Errorf("IsValidHexColor(%q) = true", c)
		}
	}
}
