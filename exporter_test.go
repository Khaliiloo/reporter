package reports

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

func testReport() *models.Report {
	return &models.Report{
		Title:  "Test Report",
		Header: models.Header{Left: "left", Right: "right"},
		Footer: models.Footer{Left: "l", Right: "r"},
		Columns: []models.Column{
			{Header: "Name", ColumnData: []models.Cell{
				{Value: "Alice", Type: models.DataTypeString},
				{Value: "Bob", Type: models.DataTypeString},
			}},
			{Header: "Score", ColumnData: []models.Cell{
				{Value: 90.5, Type: models.DataTypeNumber},
				{Value: 100.0, Type: models.DataTypeNumber},
			}},
		},
	}
}

func TestExportExcelSignature(t *testing.T) {
	out, err := Export(testReport(), FormatExcel)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	// xlsx is a ZIP archive starting with PK.
	if !bytes.HasPrefix(out, []byte("PK")) {
		t.Errorf("excel output does not start with PK magic: %x", out[:4])
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}
	var found bool
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			found = true
		}
	}
	if !found {
		t.Error("xlsx missing xl/worksheets/sheet1.xml")
	}
}

func TestExportCSV(t *testing.T) {
	out, err := Export(testReport(), FormatCSV)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	want := "Name,Score\r\nAlice,90.500\r\nBob,100.000\r\n"
	if string(out) != want {
		t.Errorf("CSV =\n%q\nwant\n%q", string(out), want)
	}
}

func TestExportPDFSignature(t *testing.T) {
	out, err := Export(testReport(), FormatPDF)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("pdf output does not start with %%PDF: %q", out[:8])
	}
}

func TestExportWordSignature(t *testing.T) {
	out, err := Export(testReport(), FormatWord)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("PK")) {
		t.Errorf("docx output does not start with PK magic")
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}
	var foundDocument bool
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			foundDocument = true
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			if !strings.Contains(buf.String(), "Test Report") {
				t.Error("document.xml missing title")
			}
			if !strings.Contains(buf.String(), "Alice") {
				t.Error("document.xml missing cell data")
			}
		}
	}
	if !foundDocument {
		t.Error("docx missing word/document.xml")
	}
}

func TestExportValidates(t *testing.T) {
	_, err := Export(&models.Report{}, FormatCSV)
	if !errors.Is(err, ErrEmptyReport) {
		t.Fatalf("expected ErrEmptyReport, got %v", err)
	}
}

func TestExportUnsupportedFormat(t *testing.T) {
	_, err := Export(testReport(), models.Format("html"))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestExportToWriterStreamsCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportToWriter(testReport(), FormatCSV, &buf); err != nil {
		t.Fatalf("ExportToWriter: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "Name,Score") {
		t.Errorf("streamed CSV = %q", buf.String())
	}
}

func TestExportToWriterPDF(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportToWriter(testReport(), FormatPDF, &buf); err != nil {
		t.Fatalf("ExportToWriter: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Errorf("pdf output does not start with %%PDF")
	}
}

func TestExportToWriterValidates(t *testing.T) {
	err := ExportToWriter(&models.Report{}, FormatCSV, &bytes.Buffer{})
	if !errors.Is(err, ErrEmptyReport) {
		t.Fatalf("expected ErrEmptyReport, got %v", err)
	}
}

func TestExportWithOptions(t *testing.T) {
	opts := DefaultOptions()
	opts.Format = FormatExcel
	opts.SheetName = "Data"
	opts.AutoSizeColumns = true
	opts.FreezeHeader = true

	out, err := ExportWithOptions(testReport(), opts)
	if err != nil {
		t.Fatalf("ExportWithOptions: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("PK")) {
		t.Error("expected xlsx output")
	}
}

func TestFactory(t *testing.T) {
	renderer, err := NewRenderer(FormatPDF)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, err := renderer.Render(testReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Error("expected pdf output")
	}
}

func TestSupportedFormatsAndMime(t *testing.T) {
	formats := SupportedFormats()
	if len(formats) != 4 {
		t.Errorf("SupportedFormats = %v", formats)
	}
	for _, f := range formats {
		if MimeType(f) == "" || FileExtension(f) == "" {
			t.Errorf("missing mime/extension for %s", f)
		}
	}
	if MimeType(FormatExcel) != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Errorf("wrong mime for excel")
	}
}

func TestExportToBytesAlias(t *testing.T) {
	a, err := Export(testReport(), FormatCSV)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExportToBytes(testReport(), FormatCSV)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("ExportToBytes differs from Export")
	}
}

// mockRenderer demonstrates the pluggable renderer contract and verifies that
// RegisterRenderer extends the factory without touching existing code.
type mockRenderer struct{ marker string }

func (m *mockRenderer) Render(report *models.Report) ([]byte, error) {
	return []byte(m.marker + ":" + report.Title), nil
}

func TestRegisterRendererExtendsFactory(t *testing.T) {
	const fmtMarkdown = models.Format("markdown")
	if err := RegisterRenderer(fmtMarkdown, func(models.ExportOptions) (Renderer, error) {
		return &mockRenderer{marker: "MD"}, nil
	}); err != nil {
		t.Fatalf("RegisterRenderer: %v", err)
	}

	r, err := NewRenderer(fmtMarkdown)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, err := r.Render(testReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "MD:Test Report" {
		t.Errorf("output = %q", out)
	}

	if err := RegisterRenderer(fmtMarkdown, nil); err == nil {
		t.Error("expected duplicate registration to fail")
	}

	if !containsFormat(SupportedFormats(), fmtMarkdown) {
		t.Error("registered format missing from SupportedFormats")
	}
}

func containsFormat(formats []models.Format, want models.Format) bool {
	for _, f := range formats {
		if f == want {
			return true
		}
	}
	return false
}

// TestConcurrentRendering verifies that a single factory-returned renderer can
// be shared safely across goroutines: renderers are immutable and all mutable
// state is per-call.
func TestConcurrentRendering(t *testing.T) {
	report := testReport()

	for _, format := range []models.Format{FormatCSV, FormatPDF, FormatWord, FormatExcel} {
		renderer, err := NewRenderer(format)
		if err != nil {
			t.Fatalf("NewRenderer(%s): %v", format, err)
		}
		baseline, err := renderer.Render(report)
		if err != nil {
			t.Fatalf("baseline render %s: %v", format, err)
		}

		const workers = 8
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := renderer.Render(report)
				if err != nil {
					errs <- err
					return
				}
				// PDF embeds a per-second CreationDate, so byte-equality would be
				// flaky; assert structural integrity instead.
				if format == FormatPDF {
					if !bytes.HasPrefix(out, []byte("%PDF")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
						errs <- fmt.Errorf("invalid pdf output")
					}
					return
				}
				if !bytes.Equal(out, baseline) {
					errs <- fmt.Errorf("concurrent output differs from baseline")
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("format %s: %v", format, err)
		}
	}
}
