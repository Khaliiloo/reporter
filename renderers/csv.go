package renderers

import (
	"bytes"
	"encoding/csv"
	"io"

	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/models"
)

// CSVOptions carries settings specific to CSV output.
type CSVOptions struct {
	// IncludeBOM writes a UTF-8 byte-order mark so Excel detects UTF-8.
	IncludeBOM bool
}

// CSVRenderer renders reports to RFC 4180 CSV. Styling is intentionally
// ignored: CSV has no styling vocabulary.
type CSVRenderer struct {
	opts    models.ExportOptions
	csvOpts CSVOptions
}

// NewCSVRenderer builds a CSVRenderer with the given export options.
func NewCSVRenderer(opts models.ExportOptions) *CSVRenderer {
	return &CSVRenderer{opts: opts}
}

// NewCSVRendererWithOptions builds a CSVRenderer with format-specific CSV
// options. Set CSVOptions.IncludeBOM when the CSV is consumed by Excel so it
// is detected as UTF-8.
func NewCSVRendererWithOptions(opts models.ExportOptions, csv CSVOptions) *CSVRenderer {
	return &CSVRenderer{opts: opts, csvOpts: csv}
}

// Render implements interfaces.Renderer.
func (r *CSVRenderer) Render(report *models.Report) ([]byte, error) {
	var buf bytes.Buffer
	if err := r.RenderToWriter(report, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderToWriter implements interfaces.WriterRenderer. The CSV is written to
// w without materializing the whole output in memory, making it suitable for
// very large datasets. The writer is not closed.
func (r *CSVRenderer) RenderToWriter(report *models.Report, w io.Writer) error {
	if err := checkConsistent(report); err != nil {
		return errors.WrapRenderError(string(models.FormatCSV), err)
	}

	if r.csvOpts.IncludeBOM {
		if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return errors.WrapRenderError(string(models.FormatCSV), err)
		}
	}

	cw := csv.NewWriter(w)
	cw.UseCRLF = true

	// Header row.
	headers := make([]string, len(report.Columns))
	for i, col := range report.Columns {
		headers[i] = col.Header
	}
	if err := cw.Write(headers); err != nil {
		return errors.WrapRenderError(string(models.FormatCSV),
			errors.WrapFormatError(string(models.FormatCSV), err))
	}

	// Data rows, streamed from the row iterator.
	it := NewRowIterator(report)
	record := make([]string, len(report.Columns))
	for it.Next() {
		row := it.Row()
		for i := range row {
			record[i] = row[i].String()
		}
		if err := cw.Write(record); err != nil {
			return errors.WrapRenderError(string(models.FormatCSV),
				errors.WrapFormatError(string(models.FormatCSV), err))
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return errors.WrapRenderError(string(models.FormatCSV),
			errors.WrapFormatError(string(models.FormatCSV), err))
	}
	return nil
}
