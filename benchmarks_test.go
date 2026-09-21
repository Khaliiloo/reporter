package reports

import (
	"bytes"
	"fmt"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

// benchReport builds a report with cols columns and rows rows.
func benchReport(cols, rows int) *models.Report {
	r := &models.Report{Title: "Benchmark Report"}
	for c := 0; c < cols; c++ {
		col := models.Column{
			Header:     fmt.Sprintf("Column %d", c),
			ColumnData: make([]models.Cell, 0, rows),
		}
		for i := 0; i < rows; i++ {
			col.ColumnData = append(col.ColumnData, models.Cell{
				Value: fmt.Sprintf("row-%d-value-%d", i, c),
				Type:  models.DataTypeString,
			})
		}
		r.Columns = append(r.Columns, col)
	}
	return r
}

func BenchmarkParse(b *testing.B) {
	var payload []byte
	{
		var buf bytes.Buffer
		buf.WriteString(`{"title":"T","data":[`)
		for c := 0; c < 5; c++ {
			if c > 0 {
				buf.WriteString(",")
			}
			fmt.Fprintf(&buf, `{"column_header":"C%d","column_data":[`, c)
			for i := 0; i < 100; i++ {
				if i > 0 {
					buf.WriteString(",")
				}
				if i%3 == 0 {
					fmt.Fprintf(&buf, `{"value":%d,"type":"number"}`, i)
				} else {
					fmt.Fprintf(&buf, `{"value":"v%d"}`, i)
				}
			}
			buf.WriteString("]}")
		}
		buf.WriteString("]}")
		payload = buf.Bytes()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidate(b *testing.B) {
	r := benchReport(5, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Validate(r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExportCSV(b *testing.B) {
	r := benchReport(5, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Export(r, FormatCSV); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExportExcel(b *testing.B) {
	r := benchReport(5, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Export(r, FormatExcel); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExportPDF(b *testing.B) {
	r := benchReport(5, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Export(r, FormatPDF); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExportWord(b *testing.B) {
	r := benchReport(5, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Export(r, FormatWord); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCSVStreamToDiscard(b *testing.B) {
	r := benchReport(5, 1000)
	discard := &discardWriter{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ExportToWriter(r, FormatCSV, discard); err != nil {
			b.Fatal(err)
		}
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
