package renderers

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func logoReport() *models.Report {
	b, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	return &models.Report{Title: "Logo Report", Logo: b, Columns: []models.Column{
		{Header: "Name", ColumnData: []models.Cell{{Value: "A", Type: models.DataTypeString}}},
	}}
}

func TestPDFLogoIsEmbeddedAndPlacedBeforeTitle(t *testing.T) {
	r := NewPDFRenderer(models.ExportOptions{})
	var order []string
	r.layoutHook = func(kind string, _, _, _, _ float64) { order = append(order, kind) }
	if _, err := r.Render(logoReport()); err != nil {
		t.Fatal(err)
	}
	if len(order) < 2 || order[0] != "logo" || order[1] != "title" {
		t.Fatalf("layout order = %v, want logo then title", order)
	}
}

func TestWordLogoIsPackagedAndReferenced(t *testing.T) {
	out, err := NewWordRenderer(models.ExportOptions{}).Render(logoReport())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	var media []byte
	for _, f := range zr.File {
		if f.Name == "word/media/logo.png" {
			rc, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			media, e = io.ReadAll(rc)
			_ = rc.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if len(media) == 0 {
		t.Fatal("logo media part missing")
	}
	var doc []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			doc, _ = io.ReadAll(rc)
			_ = rc.Close()
		}
	}
	if !strings.Contains(string(doc), `r:embed="rIdLogo"`) {
		t.Fatal("document does not reference logo relationship")
	}
}
