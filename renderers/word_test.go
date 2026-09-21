package renderers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strings"
	"testing"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

// TestWordNoTableInParagraph is a regression test for the DOCX corruption
// issue: in OOXML, w:tbl is a block-level element and must never be nested
// inside a w:p. The previous writer wrapped the header/footer tables in a
// paragraph, which Word flags as unreadable content.
func TestWordNoTableInParagraph(t *testing.T) {
	for name, part := range wordParts(t) {
		t.Run(name, func(t *testing.T) {
			assertNoTableInParagraph(t, part)
		})
	}
}

func assertNoTableInParagraph(t *testing.T, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	depthP := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("malformed XML: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "p" {
				depthP++
			}
			if el.Name.Local == "tbl" && depthP > 0 {
				t.Error("w:tbl must not be nested inside w:p")
			}
		case xml.EndElement:
			if el.Name.Local == "p" {
				depthP--
			}
		}
	}
}

// TestWordHeaderFooterTableDirectChild verifies the header/footer tables are
// direct children of w:hdr / w:ftr.
func TestWordHeaderFooterTableDirectChild(t *testing.T) {
	parts := wordParts(t)

	for _, name := range []string{"word/header1.xml", "word/footer1.xml"} {
		data := parts[name]
		root := xmlName(data, 0)
		if root != "hdr" && root != "ftr" {
			t.Fatalf("%s root = %q", name, root)
		}
		// The first child of the root must be the table.
		firstChild := xmlName(data, 1)
		if firstChild != "tbl" {
			t.Errorf("%s: first child of %s = %q, want tbl", name, root, firstChild)
		}
	}
}

// TestWordBodyStructure verifies the document body ordering: title paragraph,
// metadata paragraph with a right tab stop, separator paragraph with a bottom
// border, then the data table.
func TestWordBodyStructure(t *testing.T) {
	doc := string(wordParts(t)["word/document.xml"])

	title := pos(doc, "Test Report")
	meta := pos(doc, `<w:tabs><w:tab w:val="right"`)
	sep := pos(doc, `<w:pBdr>`)
	tbl := pos(doc, `<w:tbl>`)

	if !(title < meta && meta < sep && sep < tbl) {
		t.Errorf("bad body order: title=%d meta=%d sep=%d tbl=%d", title, meta, sep, tbl)
	}
	// Metadata text present.
	if !strings.Contains(doc, "Header Left") || !strings.Contains(doc, "Header Right") {
		t.Error("document body missing header_left/header_right metadata")
	}
}

// TestWordPackageIntegrity validates the OOXML package: every relationship
// target exists and every part is covered by [Content_Types].xml.
func TestWordPackageIntegrity(t *testing.T) {
	zr := openWordZip(t)

	entries := map[string]bool{}
	for _, f := range zr.File {
		entries["/"+f.Name] = true
	}

	// Every relationship target must exist.
	for _, relFile := range []string{"_rels/.rels", "word/_rels/document.xml.rels"} {
		data := readZip(t, zr, relFile)
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: %v", relFile, err)
			}
			if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "Relationship" {
				target := attr(start, "Target")
				resolved := resolveTarget(relFile, target)
				if !entries[resolved] {
					t.Errorf("%s references %s (%s) which does not exist", relFile, target, resolved)
				}
			}
		}
	}

	// Every part must be declared in [Content_Types].xml.
	ct := readZip(t, zr, "[Content_Types].xml")
	var defaults, overrides []string
	dec := xml.NewDecoder(bytes.NewReader([]byte(ct)))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("[Content_Types].xml: %v", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			switch start.Name.Local {
			case "Default":
				defaults = append(defaults, attr(start, "Extension"))
			case "Override":
				overrides = append(overrides, attr(start, "PartName"))
			}
		}
	}
	for f := range entries {
		if f == "/[Content_Types].xml" {
			continue
		}
		if strings.Contains(f, "/_rels/") {
			continue
		}
		covered := false
		ext := strings.TrimPrefix(path.Ext(f), ".")
		for _, e := range defaults {
			if strings.EqualFold(e, ext) {
				covered = true
				break
			}
		}
		for _, o := range overrides {
			if o == f {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("part %s is not declared in [Content_Types].xml", f)
		}
	}
}

// --- helpers ---

func wordParts(t *testing.T) map[string][]byte {
	t.Helper()
	zr := openWordZip(t)
	out := map[string][]byte{}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			out[f.Name] = readZip(t, zr, f.Name)
		}
	}
	return out
}

func openWordZip(t *testing.T) *zip.Reader {
	t.Helper()
	report := &models.Report{
		Title:  "Test Report",
		Header: models.Header{Left: "Header Left", Right: "Header Right"},
		Footer: models.Footer{Left: "Footer Left", Right: "Footer Right"},
		Columns: []models.Column{
			{Header: "Name", ColumnData: []models.Cell{
				{Value: "Alice", Type: models.DataTypeString},
			}},
			{Header: "Score", ColumnData: []models.Cell{
				{Value: 90.5, Type: models.DataTypeNumber},
			}},
		},
	}
	out, err := NewWordRenderer(models.ExportOptions{}).Render(report)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	return zr
}

func readZip(t *testing.T, zr *zip.Reader, name string) []byte {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return b
		}
	}
	t.Fatalf("entry %s not found", name)
	return nil
}

// xmlName returns the Local name of the element at the given depth, where
// depth 0 is the root element.
func xmlName(data []byte, depth int) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	cur := -1
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch tok.(type) {
		case xml.StartElement:
			cur++
			if cur == depth {
				return tok.(xml.StartElement).Name.Local
			}
		case xml.EndElement:
			cur--
		}
	}
}

func pos(s, needle string) int {
	i := strings.Index(s, needle)
	if i < 0 {
		return -1
	}
	return i
}

func attr(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// resolveTarget resolves a relationship target relative to the rels file.
func resolveTarget(relsFile, target string) string {
	if strings.HasPrefix(target, "/") {
		return target
	}
	base := strings.TrimSuffix(path.Dir(relsFile), "_rels")
	base = strings.TrimSuffix(base, "/")
	return "/" + path.Join(base, target)
}
