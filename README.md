# reports

A reusable, framework-agnostic report generation library for Go. Feed it a JSON report definition (or build the model programmatically) and export it to **Excel (.xlsx)**, **CSV**, **PDF**, or **Word (.docx)** — as bytes or streamed to an `io.Writer`.

`reports` returns output in memory or writes to a caller-provided writer, so it fits anywhere: saving to disk, uploading to object storage, streaming over HTTP, attaching to emails, or persisting in a database. It has no web-framework dependencies.

## Features

- **Four built-in formats**: Excel, CSV, PDF, and Word, with sensible defaults (auto-sized columns, frozen header row, portrait orientation).
- **Pluggable renderers**: register new formats (HTML, Markdown, XML, images, …) via `reports.RegisterRenderer` without touching existing code.
- **Two-phase pipeline**: untrusted JSON definitions are parsed and validated before any renderer sees them.
- **Streaming support**: renderers implementing `WriterRenderer` stream output directly, enabling memory-efficient exports of very large datasets.
- **Styling**: per-column and per-cell font style/size, font color, and cell background color; page header and footer; optional logo (PNG/JPEG) on document formats.
- **RTL support**: reports carry a text direction (`LTR` or `RTL`) honored at render time — including Arabic text shaping in PDFs.
- **Structured errors**: sentinel errors (`errors.Is`) and typed errors (`errors.As`) such as `ValidationError`, `RenderError`, and `FormatError`, with the offending field identified.
- **Concurrency-safe**: the renderer registry and renderer instances are safe for concurrent use.

## Installation

```
go get github.com/Khaliiloo/reporter
```

Requires Go 1.25+.

## Quick start

```go
package main

import (
	"os"

	"github.com/Khaliiloo/reporter"
)

func main() {
	jsonData := []byte(`{
		"title": "Monthly Flights",
		"header_left": "Rehlaa",
		"text_direction": "LTR",
		"data": [
			{"column_header": "Route", "column_data": [
				{"value": "CAI-DXB"}, {"value": "JED-LHR"}
			]},
			{"column_header": "Passengers", "column_data": [
				{"value": 142, "type": "number"}, {"value": 98, "type": "number"}
			]}
		]
	}`)

	report, err := reports.Parse(jsonData)
	if err != nil {
		panic(err)
	}

	xlsx, err := reports.Export(report, reports.FormatExcel)
	if err != nil {
		panic(err)
	}
	os.WriteFile("flights.xlsx", xlsx, 0o644)
}
```

### Streaming over HTTP

```go
w.Header().Set("Content-Type", reports.MimeType(reports.FormatExcel))
w.Header().Set("Content-Disposition", `attachment; filename="flights.xlsx"`)
if err := reports.ExportToWriter(report, reports.FormatExcel, w); err != nil {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
```

### Export options

```go
opts := reports.DefaultOptions()
opts.Format = reports.FormatPDF
opts.PageOrientation = reports.OrientationLandscape

pdf, err := reports.ExportWithOptions(report, opts)
```

| Option | Applies to | Description |
| --- | --- | --- |
| `SheetName` | Excel | Worksheet name (default `Sheet1`) |
| `PageOrientation` | PDF, Word | `OrientationPortrait` (default) or `OrientationLandscape` |
| `AutoSizeColumns` | Excel, PDF, Word | Estimate column widths from content (default on) |
| `FreezeHeader` | Excel | Freeze the header row (default on) |
| `CompressOutput` | xlsx/docx | Request a compressed artifact; ignored where unsupported |

### Building the model programmatically

```go
report := &reports.Report{
	Title:         "Monthly Flights",
	TextDirection: reports.TextDirectionLTR,
	Header:        reports.Header{Left: "Rehlaa", Right: "August 2026"},
	Columns: []reports.Column{{
		Header: "Route",
		ColumnData: []reports.Cell{
			{Value: "CAI-DXB", Type: reports.DataTypeString},
			{Value: "JED-LHR", Type: reports.DataTypeString},
		},
	}},
}

if err := reports.Validate(report); err != nil {
	panic(err)
}
```

### Helpers

- `reports.MimeType(format)` — standard media type for a format (handy for HTTP responses).
- `reports.FileExtension(format)` — `.xlsx`, `.csv`, `.pdf`, or `.docx`.
- `reports.SupportedFormats()` — formats with registered renderers, sorted.

## JSON definition reference

The column-major schema is the input format accepted by `reports.Parse`:

```json
{
	"title": "Monthly Flights",
	"text_direction": "LTR",
	"header_left": "Rehlaa",
	"header_right": "Page 1",
	"footer_left": "Confidential",
	"footer_right": "Generated 2026-08-19",
	"data": [
		{
			"column_header": "Passengers",
			"style": {
				"header_font_style": "bold",
				"header_font_size": 12,
				"header_font_color": "#FFFFFF",
				"header_cell_color": "#1F4E78",
				"data_font_style": "normal",
				"data_font_size": 11,
				"data_font_color": "#000000",
				"column_cell_color": "#F2F2F2"
			},
			"column_data": [
				{"value": 142, "type": "number", "font_style": "bold"},
				{"value": "2026-08-01", "type": "date"},
				{"value": true, "type": "boolean"}
			]
		}
	]
}
```

Cell `type` is one of `string`, `number`, `date`, `boolean`; `font_style` is one of `normal`, `bold`, `italic`, `underline`. Colors are hex strings like `#1F4E78`. `text_direction` is `LTR` or `RTL`. A logo can be supplied as base64-encoded PNG/JPEG bytes in the definition (see `parser.go`).

## Adding a custom format

```go
type htmlRenderer struct{ opts models.ExportOptions }

func (r *htmlRenderer) Render(report *models.Report) ([]byte, error) {
	// build the HTML document from report.Columns ...
}

if err := reports.RegisterRenderer("html", func(opts models.ExportOptions) (interfaces.Renderer, error) {
	return &htmlRenderer{opts}, nil
}); err != nil {
	panic(err)
}

html, err := reports.Export(report, "html")
```

Renderer implementations must be safe for concurrent use and may additionally implement `interfaces.WriterRenderer` to stream output. Keep them under `renderers/` and depend only on `models` and `interfaces` (never the root package) to avoid import cycles.

## Error handling

All exported errors are re-exported at the package root, so no extra imports are needed:

```go
if err := reports.Export(report, reports.FormatExcel); err != nil {
	switch {
	case errors.Is(err, reports.ErrInvalidSchema):
		// malformed JSON definition
	case errors.Is(err, reports.ErrColumnMismatch):
		// columns have unequal row counts
	case errors.Is(err, reports.ErrUnsupportedFormat):
		// no renderer registered
	}

	var verr reports.ValidationError
	if errors.As(err, &verr) {
		log.Printf("validation failed on field %q", verr.Field)
	}
}
```

## Development

```
go test ./...                 # full test suite
go test -race ./...           # race checks for registry and renderers
go test -bench . -benchmem ./ # export/parsing benchmarks
go vet ./...
gofmt -w .
```

Sample outputs generated by the test suite live in the repository root (`sample.xlsx`, `sample.csv`, `sample.pdf`, `sample.docx`); a runnable demo is in `_samples/main.go`. See [DESIGN.md](DESIGN.md) for the architecture of the parse → validate → model → render pipeline.

## License

All rights reserved by the Rehlaa project.
