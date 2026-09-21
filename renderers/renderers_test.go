package renderers

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/xml"
	stderrors "errors"
	"io"
	"strings"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/errors"
	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
	"github.com/xuri/excelize/v2"
)

func excelReport() *models.Report {
	return &models.Report{
		Title: "Round Trip",
		Columns: []models.Column{
			{Header: "Name", ColumnData: []models.Cell{
				{Value: "Alice", Type: models.DataTypeString},
				{Value: "Bob", Type: models.DataTypeString},
			}},
			{Header: "Score", ColumnData: []models.Cell{
				{Value: 90.5, Type: models.DataTypeNumber},
				{Value: 100.0, Type: models.DataTypeNumber},
			}},
			{Header: "Active", ColumnData: []models.Cell{
				{Value: true, Type: models.DataTypeBoolean},
				{Value: false, Type: models.DataTypeBoolean},
			}},
		},
	}
}

// TestExcelRoundTrip renders an xlsx and reads it back with excelize,
// verifying typed cells, values, the sheet name and frozen panes.
func TestExcelRoundTrip(t *testing.T) {
	opts := models.ExportOptions{
		SheetName:       "Data",
		AutoSizeColumns: true,
		FreezeHeader:    true,
	}
	r := NewExcelRenderer(opts)
	out, err := r.Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Data" {
		t.Errorf("sheets = %v", sheets)
	}

	if v, _ := f.GetCellValue("Data", "A1"); v != "Name" {
		t.Errorf("A1 = %q", v)
	}
	if v, _ := f.GetCellValue("Data", "A2"); v != "Alice" {
		t.Errorf("A2 = %q", v)
	}
	if v, _ := f.GetCellValue("Data", "B2"); v != "90.5" {
		t.Errorf("B2 = %q", v)
	}
	if v, _ := f.GetCellValue("Data", "C2"); v != "TRUE" {
		t.Errorf("C2 = %q", v)
	}

	// B2 must be a numeric cell, not a string.
	sheetXML := readZipEntry(t, out, "xl/worksheets/sheet1.xml")
	for _, needle := range []string{`r="B2"`, `>90.5<`} {
		if !strings.Contains(sheetXML, needle) {
			t.Errorf("sheet1.xml missing %s", needle)
		}
	}
	if strings.Contains(sheetXML, `<c r="B2" t="s"`) || strings.Contains(sheetXML, `<c r="B2" t="inlineStr"`) {
		t.Error("B2 should be a numeric cell")
	}

	// Freeze panes: the pane element references A2.
	if !strings.Contains(sheetXML, `topLeftCell="A2"`) {
		t.Error("freeze pane A2 missing")
	}
}

func TestExcelAutoWidthDisabled(t *testing.T) {
	opts := models.ExportOptions{SheetName: "S", AutoSizeColumns: false}
	r := NewExcelRenderer(opts)
	out, err := r.Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	sheetXML := readZipEntry(t, out, "xl/worksheets/sheet1.xml")
	if strings.Contains(sheetXML, `<cols`) {
		t.Error("columns should not be auto-sized when disabled")
	}
}

// TestWordXMLWellFormed verifies every XML part we generate parses cleanly,
// which guards against malformed OOXML produced by the hand-rolled writer.
func TestWordXMLWellFormed(t *testing.T) {
	opts := models.ExportOptions{PageOrientation: models.OrientationLandscape}
	out, err := NewWordRenderer(opts).Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xml") && !strings.HasSuffix(f.Name, ".rels") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			if _, err := dec.Token(); err != nil {
				if err == io.EOF {
					break
				}
				t.Errorf("malformed XML in %s: %v", f.Name, err)
				break
			}
		}
	}
}

func TestWordLandscapeOrientation(t *testing.T) {
	opts := models.ExportOptions{PageOrientation: models.OrientationLandscape}
	out, err := NewWordRenderer(opts).Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := readZipEntry(t, out, "word/document.xml")
	if !strings.Contains(doc, `w:orient="landscape"`) {
		t.Error("document.xml missing landscape orientation")
	}
}

func TestWordHeaderFooter(t *testing.T) {
	r := excelReport()
	r.Header = models.Header{Left: "HL", Right: "HR"}
	r.Footer = models.Footer{Left: "FL", Right: "FR"}
	out, err := NewWordRenderer(models.ExportOptions{}).Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	hdr := readZipEntry(t, out, "word/header1.xml")
	for _, needle := range []string{"HL", "HR"} {
		if !strings.Contains(hdr, needle) {
			t.Errorf("header1.xml missing %s", needle)
		}
	}
	ftr := readZipEntry(t, out, "word/footer1.xml")
	for _, needle := range []string{"FL", "FR", "PAGE"} {
		if !strings.Contains(ftr, needle) {
			t.Errorf("footer1.xml missing %s", needle)
		}
	}
}

func TestCSVStreamingEquality(t *testing.T) {
	r := NewCSVRenderer(models.ExportOptions{})
	b1, err := r.Render(excelReport())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.RenderToWriter(excelReport(), &buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, buf.Bytes()) {
		t.Error("Render and RenderToWriter produce different CSV")
	}
}

// TestCSVContent verifies the header row, typed data rows and CRLF line
// endings of the generated CSV.
func TestCSVContent(t *testing.T) {
	out, err := NewCSVRenderer(models.ExportOptions{}).Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("default CSV must not include a BOM")
	}
	if !bytes.Contains(out, []byte("\r\n")) {
		t.Error("CSV must use CRLF line endings (RFC 4180)")
	}

	recs, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	want := [][]string{
		{"Name", "Score", "Active"},
		{"Alice", "90.500", "true"},
		{"Bob", "100.000", "false"},
	}
	if len(recs) != len(want) {
		t.Fatalf("records = %d, want %d", len(recs), len(want))
	}
	for i := range want {
		if strings.Join(recs[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("record %d = %v, want %v", i, recs[i], want[i])
		}
	}
}

func TestCSVBOMOptIn(t *testing.T) {
	plain, err := NewCSVRenderer(models.ExportOptions{}).Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	r := NewCSVRendererWithOptions(models.ExportOptions{}, CSVOptions{IncludeBOM: true})
	out, err := r.Render(excelReport())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	bom := []byte{0xEF, 0xBB, 0xBF}
	if !bytes.HasPrefix(out, bom) {
		t.Fatal("IncludeBOM must write a UTF-8 byte-order mark")
	}
	if !bytes.Equal(out[3:], plain) {
		t.Error("BOM output must equal plain output after the byte-order mark")
	}
}

// TestCSVInvalidReport verifies structural failures surface as RenderError
// wrapping the typed validation sentinel.
func TestCSVInvalidReport(t *testing.T) {
	r := NewCSVRenderer(models.ExportOptions{})

	if _, err := r.Render(nil); !stderrors.Is(err, errors.ErrNilReport) {
		t.Errorf("nil report: err = %v, want ErrNilReport", err)
	}

	bad := excelReport()
	bad.Columns[1].ColumnData = bad.Columns[1].ColumnData[:1]
	_, err := r.Render(bad)
	if !stderrors.Is(err, errors.ErrColumnMismatch) {
		t.Errorf("mismatched columns: err = %v, want ErrColumnMismatch", err)
	}
	var re *errors.RenderError
	if !stderrors.As(err, &re) || re.Format != string(models.FormatCSV) {
		t.Errorf("mismatched columns: err = %v, want *RenderError for csv", err)
	}

	if _, err := r.Render(&models.Report{Columns: []models.Column{{Header: "A"}}}); !stderrors.Is(err, errors.ErrEmptyReport) {
		t.Errorf("no data rows: err = %v, want ErrEmptyReport", err)
	}
}

func TestPDFManyPages(t *testing.T) {
	report := &models.Report{Title: "Big"}
	for c := 0; c < 4; c++ {
		col := models.Column{Header: "Col"}
		for i := 0; i < 500; i++ {
			col.ColumnData = append(col.ColumnData, models.Cell{
				Value: strings.Repeat("x", 60), Type: models.DataTypeString,
			})
		}
		report.Columns = append(report.Columns, col)
	}
	out, err := NewPDFRenderer(models.ExportOptions{}).Render(report)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a pdf")
	}
}

// --- rows.go ---

// TestCheckConsistent covers the structural guard renderers run before
// touching cells: nil report, empty report, mismatched columns, and success.
func TestCheckConsistent(t *testing.T) {
	if err := checkConsistent(nil); !stderrors.Is(err, errors.ErrNilReport) {
		t.Errorf("nil: err = %v, want ErrNilReport", err)
	}
	// A zero-column report is not this guard's concern: per its contract it
	// only checks row consistency, and schema validation belongs to the root
	// package's Validate.
	if err := checkConsistent(&models.Report{}); err != nil {
		t.Errorf("no columns: err = %v, want nil", err)
	}
	if err := checkConsistent(&models.Report{Columns: []models.Column{{Header: "A"}}}); !stderrors.Is(err, errors.ErrEmptyReport) {
		t.Errorf("no data rows: err = %v, want ErrEmptyReport", err)
	}
	mismatch := &models.Report{Columns: []models.Column{
		{Header: "A", ColumnData: []models.Cell{{Value: "1", Type: models.DataTypeString}}},
		{Header: "B"},
	}}
	if err := checkConsistent(mismatch); !stderrors.Is(err, errors.ErrColumnMismatch) {
		t.Errorf("mismatch: err = %v, want ErrColumnMismatch", err)
	}
	if err := checkConsistent(excelReport()); err != nil {
		t.Errorf("valid report: err = %v, want nil", err)
	}
}

// TestRowIteratorTransposesAndReusesBuffer verifies the column-major report is
// iterated row by row and that Row reuses its backing buffer, as documented.
func TestRowIteratorTransposesAndReusesBuffer(t *testing.T) {
	it := NewRowIterator(excelReport())

	want := [][]string{
		{"Alice", "90.500", "true"},
		{"Bob", "100.000", "false"},
	}
	var first []models.Cell
	rows := 0
	for it.Next() {
		row := it.Row()
		if first == nil {
			first = row
		} else if &row[0] != &first[0] {
			t.Error("Row must reuse its backing buffer between iterations")
		}
		if rows >= len(want) {
			t.Fatalf("iterated row %d, want only %d", rows+1, len(want))
		}
		for j, c := range row {
			if got := c.String(); got != want[rows][j] {
				t.Errorf("row %d col %d = %q, want %q", rows, j, got, want[rows][j])
			}
		}
		rows++
	}
	if rows != len(want) {
		t.Errorf("rows = %d, want %d", rows, len(want))
	}
	if it.Next() {
		t.Error("iterator must stay exhausted after the last row")
	}
}

// TestRowIteratorEmptyColumnGuard verifies a column without data yields empty
// string cells instead of panicking on the transpose.
func TestRowIteratorEmptyColumnGuard(t *testing.T) {
	report := &models.Report{Columns: []models.Column{
		{Header: "A", ColumnData: []models.Cell{{Value: "x", Type: models.DataTypeString}}},
		{Header: "B"},
	}}
	it := NewRowIterator(report)
	if !it.Next() {
		t.Fatal("expected one row")
	}
	row := it.Row()
	if len(row) != 2 {
		t.Fatalf("row length = %d, want 2", len(row))
	}
	if row[1].Value != "" || row[1].Type != models.DataTypeString {
		t.Errorf("empty column cell = %+v, want empty string cell", row[1])
	}
	if it.Next() {
		t.Error("row count must derive from the first non-empty column")
	}
}

// --- text.go ---

func TestIsArabic(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"مرحبا", true},
		{"hello", false},
		{"mixed مرحبا text", true},
		{"123 !@#", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsArabic(c.in); got != c.want {
			t.Errorf("IsArabic(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestNormalizeTextAppliesNFC verifies decomposed sequences are composed so
// renderers compare and measure canonical runes.
func TestNormalizeTextAppliesNFC(t *testing.T) {
	decomposed := "e\u0301" // e + combining acute
	if got := NormalizeText(decomposed); got != "\u00e9" {
		t.Errorf("NormalizeText(%q) = %q, want precomposed é", decomposed, got)
	}
	if got := NormalizeText("plain ASCII"); got != "plain ASCII" {
		t.Errorf("NormalizeText must be identity on normalized input, got %q", got)
	}
	if got := ShapeText(decomposed); got != "\u00e9" {
		t.Errorf("ShapeText(%q) = %q, want normalized output", decomposed, got)
	}
}

// --- image.go ---

// TestVisualTextPassthrough verifies BiDi reordering only applies to
// non-empty RTL text.
func TestVisualTextPassthrough(t *testing.T) {
	if got := visualText("hello", models.TextDirectionLTR); got != "hello" {
		t.Errorf("LTR text must pass through unchanged, got %q", got)
	}
	if got := visualText("", models.TextDirectionRTL); got != "" {
		t.Errorf("empty text must pass through unchanged, got %q", got)
	}
}

func TestDecodeLogo(t *testing.T) {
	if img, err := decodeLogo(nil); img != nil || err != nil {
		t.Errorf("decodeLogo(nil) = (%v, %v), want (nil, nil)", img, err)
	}
	if img, err := decodeLogo([]byte{}); img != nil || err != nil {
		t.Errorf("decodeLogo(empty) = (%v, %v), want (nil, nil)", img, err)
	}
	if _, err := decodeLogo([]byte("not an image")); err == nil {
		t.Error("garbage bytes must fail decoding")
	}

	pngBytes, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodeLogo(pngBytes)
	if err != nil {
		t.Fatalf("decodeLogo(valid PNG): %v", err)
	}
	if img == nil || img.format != "png" || img.width != 1 || img.height != 1 {
		t.Errorf("decoded logo = %+v, want 1x1 png", img)
	}
	if !bytes.Equal(img.data, pngBytes) {
		t.Error("decoded logo must keep the original bytes for embedding")
	}
}

func readZipEntry(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatalf("entry %s not found", name)
	return ""
}
