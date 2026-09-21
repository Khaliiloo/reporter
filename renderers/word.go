package renderers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/models"
)

const (
	// A4 in twentieths of a point (twips).
	a4WidthTwips  = 11906
	a4HeightTwips = 16838
	marginTwips   = 1417
)

// WordRenderer renders reports to .docx (Office Open XML). The OOXML package
// is produced directly (no third-party DOCX dependency), keeping the library
// license-clean while supporting headers, footers, tables, styling and page
// orientation.
type WordRenderer struct {
	opts models.ExportOptions
}

// NewWordRenderer builds a WordRenderer with the given export options.
func NewWordRenderer(opts models.ExportOptions) *WordRenderer {
	return &WordRenderer{opts: opts}
}

// Render implements interfaces.Renderer.
func (r *WordRenderer) Render(report *models.Report) ([]byte, error) {
	if err := checkConsistent(report); err != nil {
		return nil, errors.WrapRenderError(string(models.FormatWord), err)
	}
	logo, err := decodeLogo(report.Logo)
	if err != nil {
		return nil, errors.WrapRenderError(string(models.FormatWord), err)
	}

	d := &wordDoc{report: report, opts: r.opts, logo: logo}
	d.initLayout()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct {
		name string
		xml  string
	}{
		{"[Content_Types].xml", d.contentTypes()},
		{"_rels/.rels", d.rootRels()},
		{"docProps/core.xml", d.coreProps()},
		{"docProps/app.xml", d.appProps()},
		{"word/document.xml", d.document()},
		{"word/styles.xml", d.styles()},
		// Retained as a detached compatibility part for consumers that inspect
		// the package; document.xml intentionally does not reference it, so
		// report metadata is not duplicated as a page header.
		{"word/header1.xml", d.headerPart()},
		{"word/footer1.xml", d.footerPart()},
		{"word/_rels/document.xml.rels", d.documentRels()},
	}
	for _, p := range parts {

		if err := validateXML(p.name, p.xml); err != nil {
			return nil, err
		}
		hdr := &zip.FileHeader{Name: p.name, Method: zip.Deflate}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, errors.WrapRenderError(string(models.FormatWord),
				errors.WrapFormatError(string(models.FormatWord), err))
		}
		if _, err := w.Write([]byte(p.xml)); err != nil {
			return nil, errors.WrapRenderError(string(models.FormatWord),
				errors.WrapFormatError(string(models.FormatWord), err))
		}
	}
	if d.logo != nil {
		hdr := &zip.FileHeader{Name: "word/media/logo." + d.logo.format, Method: zip.Deflate}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, errors.WrapRenderError(string(models.FormatWord), err)
		}
		if _, err := w.Write(d.logo.data); err != nil {
			return nil, errors.WrapRenderError(string(models.FormatWord), err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, errors.WrapRenderError(string(models.FormatWord),
			errors.WrapFormatError(string(models.FormatWord), err))
	}

	return buf.Bytes(), nil
}

type wordDoc struct {
	report    *models.Report
	opts      models.ExportOptions
	landscape bool
	pageW     int
	pageH     int
	usableW   int
	colWidths []int
	logo      *logoImage
	columns   []models.Column
}

func (d *wordDoc) initLayout() {
	d.pageW, d.pageH = a4WidthTwips, a4HeightTwips
	d.landscape = d.opts.PageOrientation == models.OrientationLandscape
	if d.landscape {
		d.pageW, d.pageH = d.pageH, d.pageW
	}
	d.usableW = d.pageW - 2*marginTwips
	d.columns = renderColumns(d.report)
	d.computeColumnWidths()
}

func (d *wordDoc) computeColumnWidths() {
	n := len(d.columns)
	d.colWidths = make([]int, n)

	weights := make([]float64, n)
	for ci, col := range d.columns {
		if !d.opts.AutoSizeColumns {
			weights[ci] = 1
			continue
		}
		units := float64(len([]rune(col.Header)))
		for ri := range col.ColumnData {
			if u := float64(len([]rune(col.ColumnData[ri].String()))); u > units {
				units = u
			}
		}
		weights[ci] = units + 2
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		total = 1
	}

	used := 0
	for ci, w := range weights {
		width := int(math.Round(float64(d.usableW) * w / total))
		if width < 720 { // ~0.5 inch minimum
			width = 720
		}
		d.colWidths[ci] = width
		used += width
	}
	// Compensate rounding drift so the grid sums exactly to the usable width.
	if diff := d.usableW - used; len(d.colWidths) > 0 {
		d.colWidths[len(d.colWidths)-1] += diff
		if d.colWidths[len(d.colWidths)-1] < 1 {
			d.colWidths[len(d.colWidths)-1] = 1
		}
	}
}

func (d *wordDoc) document() string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">`)
	b.WriteString(`<w:body>`)

	if d.logo != nil {
		b.WriteString(d.logoParagraph())
	}

	// Title paragraph.
	if d.report.Title != "" {
		b.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="260"/>`)
		if d.report.TextDirection == models.TextDirectionRTL {
			b.WriteString(`<w:bidi/>`)
		}
		b.WriteString(`</w:pPr>`)
		b.WriteString(`<w:r><w:rPr><w:b/><w:sz w:val="44"/><w:szCs w:val="44"/><w:color w:val="07449e"/></w:rPr>`)
		b.WriteString(`<w:t xml:space="preserve">`)
		b.WriteString(esc(d.report.Title))
		b.WriteString(`</w:t></w:r></w:p>`)
	}

	// Report metadata line: header_left on the left, header_right on the right
	// via a right-aligned tab stop.
	if d.report.Header.Left != "" || d.report.Header.Right != "" {
		b.WriteString(`<w:p><w:pPr>`)
		b.WriteString(fmt.Sprintf(`<w:tabs><w:tab w:val="right" w:pos="%d"/></w:tabs>`, d.usableW))
		b.WriteString(`<w:spacing w:before="60" w:after="120"/></w:pPr>`)
		b.WriteString(d.runWithText(d.report.Header.Left, "", 10, "555555"))
		b.WriteString(`<w:r><w:tab/></w:r>`)
		b.WriteString(d.runWithText(d.report.Header.Right, "", 10, "555555"))
		b.WriteString(`</w:p>`)
	}

	// Separator rule below the metadata.
	b.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="8" w:space="1" w:color="auto"/></w:pBdr>`)
	b.WriteString(`<w:spacing w:before="120" w:after="160"/></w:pPr></w:p>`)

	// Table.
	b.WriteString(`<w:tbl><w:tblPr>`)
	b.WriteString(`<w:tblW w:w="0" w:type="auto"/>`)
	b.WriteString(`<w:tblBorders>`)
	for _, side := range []string{"top", "bottom"} {
		b.WriteString(`<w:`)
		b.WriteString(side)
		b.WriteString(` w:val="single" w:sz="4" w:space="0" w:color="A5C6CD"/>`)
	}
	for _, side := range []string{"left", "right", "insideH", "insideV"} {
		b.WriteString(`<w:`)
		b.WriteString(side)
		b.WriteString(` w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	}
	b.WriteString(`</w:tblBorders><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	for _, w := range d.colWidths {
		b.WriteString(`<w:gridCol w:w="`)
		b.WriteString(strconv.Itoa(w))
		b.WriteString(`"/>`)
	}
	b.WriteString(`</w:tblGrid>`)

	// Header row.
	b.WriteString(`<w:tr><w:trPr><w:tblHeader/></w:trPr>`)
	for ci, col := range d.columns {
		bg := col.Style.HeaderCellColor
		if bg == "" {
			bg = "07449e"
		}
		b.WriteString(d.cellXML(col.Header, true, col.Style.HeaderFontStyle,
			fontPt(col.Style.HeaderFontSize, defaultHeaderFontSize),
			col.Style.HeaderFontColor, bg, d.colWidths[ci], models.DataTypeString))
	}
	b.WriteString(`</w:tr>`)

	// Data rows.
	it := NewRowIterator(d.report)
	for it.Next() {
		cells := renderCells(d.report, it.Row())
		b.WriteString(`<w:tr>`)
		for ci, cell := range cells {
			col := d.columns[ci]
			style := cell.FontStyle
			if style == models.FontNormal {
				style = col.Style.DataFontStyle
			}
			size := cell.FontSize
			if size <= 0 {
				size = col.Style.DataFontSize
			}
			bg := cell.CellColor
			if bg == "" {
				bg = col.Style.ColumnCellColor
			}
			b.WriteString(d.cellXML(cell.String(), false, style,
				fontPt(size, defaultDataFontSize),
				col.Style.DataFontColor, bg, d.colWidths[ci], cell.Type))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)

	// Section properties: orientation, margins, header/footer references.
	orient := ""
	if d.landscape {
		orient = ` w:orient="landscape"`
	}
	b.WriteString(`<w:sectPr>`)
	b.WriteString(`<w:headerReference w:type="default" r:id="rIdHdr"/>`)
	b.WriteString(`<w:footerReference w:type="default" r:id="rIdFtr"/>`)
	b.WriteString(fmt.Sprintf(`<w:pgSz w:w="%d" w:h="%d"%s/>`, d.pageW, d.pageH, orient))
	b.WriteString(fmt.Sprintf(`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="708" w:footer="708" w:gutter="0"/>`,
		marginTwips, marginTwips, marginTwips, marginTwips))
	b.WriteString(`</w:sectPr>`)

	b.WriteString(`</w:body></w:document>`)
	return b.String()
}

// cellXML renders a table cell with its run properties.
func (d *wordDoc) cellXML(text string, isHeader bool, fontStyle models.FontStyle, size int, color, bg string, width int, dataType models.DataType) string {
	var b strings.Builder
	b.WriteString(`<w:tc><w:tcPr>`)
	b.WriteString(fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, width))
	if bg != "" {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="`)
		b.WriteString(trimHash(bg))
		b.WriteString(`"/>`)
	}
	b.WriteString(`<w:vAlign w:val="center"/></w:tcPr>`)

	align := "left"
	if isHeader {
		align = "center"
	} else if dataType == models.DataTypeNumber && d.report.TextDirection != models.TextDirectionRTL {
		align = "right"
	} else if dataType == models.DataTypeBoolean {
		align = "center"
	}
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="40" w:after="40"/><w:jc w:val="`)
	b.WriteString(align)
	b.WriteString(`"/>`)
	if d.report.TextDirection == models.TextDirectionRTL {
		b.WriteString(`<w:bidi/>`)
	}
	b.WriteString(`</w:pPr>`)

	// Runs with line breaks for multi-line text.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteString(`<w:r><w:br/></w:r>`)
		}
		b.WriteString(`<w:r>`)
		b.WriteString(d.runProps(fontStyle, size, color))
		b.WriteString(`<w:t xml:space="preserve">`)
		b.WriteString(esc(line))
		b.WriteString(`</w:t></w:r>`)
	}

	b.WriteString(`</w:p></w:tc>`)
	return b.String()
}

// runProps renders the <w:rPr> run properties for a font directive.
func (d *wordDoc) runProps(fontStyle models.FontStyle, size int, color string) string {
	var b strings.Builder
	b.WriteString(`<w:rPr>`)
	if d.report.TextDirection == models.TextDirectionRTL {
		b.WriteString(`<w:rtl/>`)
	}
	switch fontStyle {
	case models.FontBold:
		b.WriteString(`<w:b/>`)
	case models.FontItalic:
		b.WriteString(`<w:i/>`)
	case models.FontUnderline:
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if size > 0 {
		half := size * 2
		b.WriteString(fmt.Sprintf(`<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, half, half))
	}
	if color != "" {
		b.WriteString(`<w:color w:val="`)
		b.WriteString(trimHash(color))
		b.WriteString(`"/>`)
	}
	b.WriteString(`<w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/>`)
	b.WriteString(`</w:rPr>`)
	return b.String()
}

// runWithText builds a single run with the given font directive, size (pt)
// and color, containing text.
func (d *wordDoc) runWithText(text string, fontStyle models.FontStyle, size int, color string) string {
	var b strings.Builder
	b.WriteString(`<w:r>`)
	b.WriteString(d.runProps(fontStyle, size, color))
	b.WriteString(`<w:t xml:space="preserve">`)
	b.WriteString(esc(text))
	b.WriteString(`</w:t></w:r>`)
	return b.String()
}

// logoParagraph embeds a proportionally scaled inline DrawingML image. The
// image relationship targets the in-memory media part written by Render.
func (d *wordDoc) logoParagraph() string {
	const maxCX, maxCY int64 = 1224000, 576000 // 1.34 by 0.63 inches
	cx, cy := maxCX, maxCX*int64(d.logo.height)/int64(d.logo.width)
	if cy > maxCY {
		cy = maxCY
		cx = maxCY * int64(d.logo.width) / int64(d.logo.height)
	}
	return fmt.Sprintf(`<w:p><w:pPr><w:jc w:val="right"/><w:spacing w:after="180"/></w:pPr><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="1" name="Report logo"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="logo.%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdLogo"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, cx, cy, d.logo.format, cx, cy)
}

func (d *wordDoc) headerPart() string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	// A table is a block-level element and must be a direct child of w:hdr.
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/>`)
	b.WriteString(`<w:tblBorders><w:top w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:left w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:bottom w:val="single" w:sz="6" w:space="1" w:color="auto"/>`)
	b.WriteString(`<w:right w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:insideH w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:insideV w:val="none" w:sz="0" w:space="0" w:color="auto"/></w:tblBorders>`)
	b.WriteString(`<w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	b.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/><w:gridCol w:w="%d"/>`, d.usableW/2, d.usableW-d.usableW/2))
	b.WriteString(`</w:tblGrid><w:tr>`)
	b.WriteString(d.headerFooterCell(d.report.Header.Left, "left", d.usableW/2))
	b.WriteString(d.headerFooterCell(d.report.Header.Right, "right", d.usableW-d.usableW/2))
	b.WriteString(`</w:tr></w:tbl></w:hdr>`)
	return b.String()
}

func (d *wordDoc) footerPart() string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	// A table is a block-level element and must be a direct child of w:ftr.
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/>`)
	b.WriteString(`<w:tblBorders><w:top w:val="single" w:sz="6" w:space="1" w:color="auto"/>`)
	b.WriteString(`<w:left w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:bottom w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:right w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:insideH w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	b.WriteString(`<w:insideV w:val="none" w:sz="0" w:space="0" w:color="auto"/></w:tblBorders>`)
	b.WriteString(`<w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	third := d.usableW / 3
	b.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/><w:gridCol w:w="%d"/><w:gridCol w:w="%d"/>`,
		third, third, d.usableW-2*third))
	b.WriteString(`</w:tblGrid><w:tr>`)
	b.WriteString(d.headerFooterCell(d.report.Footer.Left, "left", third))
	// Page number field in the middle cell.
	b.WriteString(`<w:tc><w:tcPr>`)
	b.WriteString(fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, third))
	b.WriteString(`</w:tcPr>`)
	b.WriteString(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr>`)
	b.WriteString(`<w:r><w:fldChar w:fldCharType="begin"/></w:r>`)
	b.WriteString(`<w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r>`)
	b.WriteString(`<w:r><w:fldChar w:fldCharType="end"/></w:r>`)
	b.WriteString(`</w:p></w:tc>`)
	b.WriteString(d.headerFooterCell(d.report.Footer.Right, "right", d.usableW-2*third))
	b.WriteString(`</w:tr></w:tbl></w:ftr>`)
	return b.String()
}

func (d *wordDoc) headerFooterCell(text, align string, width int) string {
	var b strings.Builder
	b.WriteString(`<w:tc><w:tcPr>`)
	b.WriteString(fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, width))
	b.WriteString(`</w:tcPr>`)
	b.WriteString(`<w:p><w:pPr><w:jc w:val="`)
	b.WriteString(align)
	b.WriteString(`"/></w:pPr>`)
	b.WriteString(`<w:r><w:rPr><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr>`)
	b.WriteString(`<w:t xml:space="preserve">`)
	b.WriteString(esc(text))
	b.WriteString(`</w:t></w:r>`)
	b.WriteString(`</w:p></w:tc>`)
	return b.String()
}

func (d *wordDoc) contentTypes() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Default Extension="png" ContentType="image/png"/>` +
		`<Default Extension="jpg" ContentType="image/jpeg"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>` +
		`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		`</Types>`
}

func (d *wordDoc) rootRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
		`</Relationships>`
}

func (d *wordDoc) documentRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`<Relationship Id="rIdHdr" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>` +
		`<Relationship Id="rIdFtr" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` +
		func() string {
			if d.logo != nil {
				return `<Relationship Id="rIdLogo" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/logo.` + d.logo.format + `"/>`
			}
			return ""
		}() +
		`</Relationships>`
}

func (d *wordDoc) styles() string {
	return xmlDecl +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr>` +
		`<w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/><w:szCs w:val="22"/>` +
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
		`</w:styles>`
}

func (d *wordDoc) coreProps() string {
	title := esc(d.report.Title)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + title + `</dc:title>` +
		`<dc:creator>Rehlaa reports</dc:creator>` +
		`</cp:coreProperties>`
}

func (d *wordDoc) appProps() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>Rehlaa reports</Application>` +
		`</Properties>`
}

const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

// esc XML-escapes text content.
func esc(s string) string {
	s = sanitizeXMLText(s)

	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))

	return b.String()
}

func sanitizeXMLText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x9:
			return r
		case r == 0xA:
			return r
		case r == 0xD:
			return r
		case r >= 0x20 && r <= 0xD7FF:
			return r
		case r >= 0xE000 && r <= 0xFFFD:
			return r
		case r >= 0x10000 && r <= 0x10FFFF:
			return r
		default:
			return -1
		}
	}, s)
}

// trimHash removes a leading '#' from hex colors for OOXML attributes.
func trimHash(s string) string {
	return strings.TrimPrefix(s, "#")
}

// fontPt returns a valid font size, falling back to def when unset.
func fontPt(size, def int) int {
	if size <= 0 {
		return def
	}
	return size
}

func validateXML(name, content string) error {
	decoder := xml.NewDecoder(strings.NewReader(content))

	for {
		if _, err := decoder.Token(); err != nil {
			if err.Error() == "EOF" {
				return nil
			}

			return fmt.Errorf("%s: %w", name, err)
		}
	}
}
