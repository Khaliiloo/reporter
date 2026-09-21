package renderers

import (
	"archive/zip"
	"bytes"
	"io"
	"log"
	"strings"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

func rtlReport() *models.Report {
	return &models.Report{Title: "تقرير المبيعات", TextDirection: models.TextDirectionRTL, Columns: []models.Column{
		{Header: "العميل", ColumnData: []models.Cell{{Value: "أحمد", Type: models.DataTypeString}}},
		{Header: "الإيرادات", ColumnData: []models.Cell{{Value: 750, Type: models.DataTypeNumber}}},
		{Header: "الإجمالي", ColumnData: []models.Cell{{Value: 1000, Type: models.DataTypeNumber}}},
	}}
}

func TestRTLRenderingReversesOnlyTheView(t *testing.T) {
	r := rtlReport()
	cols := renderColumns(r)
	if cols[0].Header != "الإجمالي" || cols[2].Header != "العميل" {
		t.Fatalf("rendered headers = %q, %q, %q", cols[0].Header, cols[1].Header, cols[2].Header)
	}
	if r.Columns[0].Header != "العميل" || r.Columns[2].Header != "الإجمالي" {
		t.Fatal("RTL rendering mutated the source report")
	}
	if got := visualText("مرحبا بالعالم", models.TextDirectionRTL); got == "مرحبا بالعالم" {
		t.Fatal("RTL text was not reordered for the PDF text API")
	}
}

func TestWordRTLUsesNativeOpenXMLDirection(t *testing.T) {
	out, err := NewWordRenderer(models.ExportOptions{}).Render(rtlReport())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	var doc []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			doc, e = io.ReadAll(rc)
			_ = rc.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	s := string(doc)
	log.Println(s)
	for _, needle := range []string{`<w:bidi/>`, `<w:rtl/>`, "الإجمالي", "العميل"} {
		if !strings.Contains(s, needle) {
			t.Errorf("document.xml missing %s", needle)
		}
	}
}
