# Rehlaa `reports` — Enterprise Report Export Library

A reusable, framework-agnostic Go module that accepts a **JSON report definition**
(or a programmatically built model) and exports it to **Excel (.xlsx)**, **CSV (.csv)**,
**PDF (.pdf)** and **Word (.docx)**.

It is designed to be imported by many independent backend applications (Gin, Echo,
Fiber, Chi, gRPC services, background workers, serverless functions, batch jobs)
**without modification**. It never touches disk itself — it returns `[]byte` or streams
to an `io.Writer`, so callers can save files, upload to S3/Azure Blob, stream over HTTP,
attach to emails, or persist to a database.

This document covers all fifteen requested deliverables.

---

## 1. Architecture Design

### 1.1 Design principles

The library separates five concerns into distinct phases, each with a single
responsibility (SOLID):

| Phase | Responsibility | Package |
|---|---|---|
| **Parsing** | JSON schema → internal model, with type coercion | root `reports` (`parser.go`) |
| **Validation** | Semantic checks of a model, typed errors | root `reports` (`validator.go`) |
| **Internal Model** | Format-agnostic domain types | `models` |
| **Rendering** | Model → output bytes, per format | `renderers` |
| **Export API** | Orchestration, factory, options, MIME helpers | root `reports` (`exporter.go`, `report.go`) |

Key decisions:

- **Open/Closed**: new formats are added via `RegisterRenderer` without touching
  existing code (factory + registry pattern).
- **Dependency Inversion**: renderers depend on the `interfaces.Renderer` contract and
  `models`, never on the root package (avoids import cycles).
- **Liskov Substitution**: every renderer implements the same `Render(report)` contract;
  an optional `WriterRenderer` extension provides streaming without changing the base
  interface.
- **Interface Segregation**: the minimal `Renderer` interface plus an optional
  `WriterRenderer`; consumers choose what they need.
- **Single Responsibility**: the CSV renderer ignores styling (CSV has no styling
  vocabulary), Excel/PDF/Word apply it; no renderer knows about JSON or validation.

### 1.2 Rendering pipeline

```
 JSON bytes ──► Parse ──► *models.Report ──► Validate ──► NewRenderer(format, opts)
                                                        │
                                              RendererFactory (registry)
                                                        │
                                              ExcelRenderer / CSVRenderer /
                                              PDFRenderer / WordRenderer
                                                        │
                                                Render()  ──► []byte
                                                RenderToWriter() ──► io.Writer
```

### 1.3 Concurrency model

Renderers are immutable (they hold only resolved `ExportOptions`), so a single
instance returned by the factory is safe for concurrent `Render`/`RenderToWriter` calls.
All mutable state (e.g. the fpdf document) is created inside `Render`, per call.
The renderer registry is guarded by a `sync.RWMutex`.

---

## 2. Package Structure

```
reports/
├── go.mod                    module github.com/Khaliiloo/reporter  (Go ≥ 1.25)
├── go.sum
├── DESIGN.md
├── report.go                 package doc, type aliases, re-exported sentinels
├── parser.go                 Parse / ParseReader — JSON schema → model
├── validator.go              Validate + option validation + hex color helper
├── exporter.go               DefaultOptions, registry, RegisterRenderer,
│                             NewRenderer(WithOptions), Export*, MimeType, ...
├── errors/
│   └── errors.go             sentinel errors + ValidationError/RenderError/FormatError
├── models/
│   ├── report.go             Report, Column, ColumnStyle, Cell, Header, Footer,
│   │                         Format, DataType, FontStyle, Orientation
│   └── options.go            ExportOptions
├── interfaces/
│   └── renderer.go           Renderer, WriterRenderer
└── renderers/
    ├── rows.go               RowIterator (streaming column→row transposition)
    ├── csv.go                CSVRenderer (stdlib encoding/csv)
    ├── excel.go              ExcelRenderer (excelize StreamWriter)
    ├── pdf.go                PDFRenderer (go-pdf/fpdf)
    ├── word.go               WordRenderer (hand-rolled OOXML zip writer)
    ├── fonts.go              embedded DejaVu Unicode fonts (go:embed)
    ├── fonts/                DejaVuSans*.ttf (Bitstream Vera license)
    ├── csv.go / excel.go / pdf.go / word.go / rows.go
    └── *_test.go             renderer tests, round-trips, XML well-formedness
```

The root package re-exports models, interfaces and error sentinels so consumers use one
import path: `github.com/Khaliiloo/reporter`.

---

## 3. Domain Models

Defined in `models/report.go`. The model is **column-major** (mirroring the JSON
schema) and independent of output formats.

| Type | Fields | Notes |
|---|---|---|
| `Report` | `Title string`, `TextDirection`, `Logo []byte`, `Header`, `Footer`, `Columns []Column` | canonical model; direction defaults to LTR |
| `Header` / `Footer` | `Left`, `Right string` | page-level text |
| `Column` | `Header string`, `Style ColumnStyle`, `ColumnData []Cell` | one data column |
| `ColumnStyle` | `HeaderFontStyle/DataFontStyle FontStyle`, `HeaderFontSize/DataFontSize int`, `HeaderFontColor/DataFontColor string`, `ColumnCellColor string` | column-level styling |
| `Cell` | `Value any`, `Type DataType`, `FontStyle FontStyle`, `FontSize int`, `CellColor string` | single cell |
| `DataType` | `string | number | date | boolean` | logical cell type |
| `FontStyle` | `normal | bold | italic | underline` | font directive |
| `Format` | `excel | csv | pdf | word` | output format |
| `ExportOptions` | `Format`, `SheetName`, `PageOrientation`, `CompressOutput`, `AutoSizeColumns`, `FreezeHeader` | per-export settings |

```go
type Report struct {
    Title   string
    Header  Header
    Footer  Footer
    Columns []Column
}

func (r *Report) RowCount() int     // derived from first non-empty column
func (r *Report) ColumnCount() int
```

**Data transformation rule.** The schema stores data per column; renderers transpose to
rows via `renderers.RowIterator`, which streams one row at a time without materializing
the full row matrix:

```
Name  [John, Jane]     ──►  | Name | Age |     ──►  | Name | Age |
Age   [25,   30 ]            | John | 25  |          | Jane | 30  |
```

---

## 4. Public API Definitions

All entry points live in the root package; identifiers from `models`, `interfaces` and
`errors` are re-exported for a single import.

```go
// Parsing
func Parse(data []byte) (*models.Report, error)
func ParseReader(r io.Reader) (*models.Report, error)

// Validation
func Validate(report *models.Report) error
func IsValidHexColor(s string) bool

// Rendering factory
func NewRenderer(format models.Format) (interfaces.Renderer, error)
func NewRendererWithOptions(format models.Format, opts models.ExportOptions) (interfaces.Renderer, error)
func RegisterRenderer(format models.Format, factory RendererFactory) error

// Export API
func Export(report *models.Report, format models.Format) ([]byte, error)
func ExportToBytes(report *models.Report, format models.Format) ([]byte, error) // alias
func ExportWithOptions(report *models.Report, opts models.ExportOptions) ([]byte, error)
func ExportToWriter(report *models.Report, format models.Format, w io.Writer) error
func ExportToWriterWithOptions(report *models.Report, opts models.ExportOptions, w io.Writer) error

// Options & metadata
func DefaultOptions() models.ExportOptions
func SupportedFormats() []models.Format
func MimeType(format models.Format) string
func FileExtension(format models.Format) string

// Re-exported types & constants
type Renderer = interfaces.Renderer
type WriterRenderer = interfaces.WriterRenderer
type Report, Column, ColumnStyle, Cell, Header, Footer, ExportOptions,
     Format, DataType, FontStyle = models.<...>
const FormatExcel, FormatCSV, FormatPDF, FormatWord, ... = models.<...>
var  ErrInvalidColor, ErrInvalidFont, ErrColumnMismatch, ... = errors.<...>
```

The library returns `[]byte` (or streams via `ExportToWriter` / `WriterRenderer`); it
never writes to disk.

---

## 5. Renderer Interfaces

```go
// interfaces/renderer.go
type Renderer interface {
    Render(report *models.Report) ([]byte, error)
}

type WriterRenderer interface {
    Renderer
    RenderToWriter(report *models.Report, w io.Writer) error
}
```

Implementations: `ExcelRenderer`, `CSVRenderer`, `PDFRenderer`, `WordRenderer`.
Consumers never see implementation details; they only see `interfaces.Renderer`.

---

## 6. Factory Implementation Design

```go
type RendererFactory func(opts models.ExportOptions) (interfaces.Renderer, error)

var rendererRegistry = map[models.Format]RendererFactory{ /* excel, csv, pdf, word */ }

func RegisterRenderer(format models.Format, factory RendererFactory) error // guarded by RWMutex
func NewRendererWithOptions(format models.Format, opts models.ExportOptions) (interfaces.Renderer, error)
```

- `NewRenderer(format)` resolves `format` with `DefaultOptions()`.
- Unset option fields are merged with defaults (`mergeOptions`).
- Options are validated (`validateOptions`) before construction.
- Custom formats are supported: `RegisterRenderer(models.Format("markdown"), factory)`.
  The extension point means **HTML, Markdown, PowerPoint, XML, JSON, images**, etc. can be
  added by consumers without forking the library.

---

## 7. Validation Flow

`Validate` (called automatically by every `Export*` function) checks, in order:

1. nil report → `ErrNilReport`
2. ≥ 1 column, else `ErrEmptyReport`
3. column data length consistency across all columns → `ErrColumnMismatch`
4. ≥ 1 data row, else `ErrEmptyReport`
5. per column: non-empty `column_header` → `ErrInvalidSchema`; font directives → `ErrInvalidFont`;
   font sizes in `[6..100]` → `ErrInvalidFontSize`; hex colors `#RGB`/`#RRGGBB` → `ErrInvalidColor`
6. per cell: type in `{string,number,date,boolean}` → `ErrInvalidType`; font/style/size/color checks

Unset values (`""`, `0`) mean "renderer default applies" and are not errors.
`ExportOptions` are validated at factory time (orientation, etc.); unknown formats are
allowed by option validation and rejected by the registry lookup (so custom formats work).

Errors are typed and locatable:

```go
var (
    ErrInvalidColor     // malformed hex color
    ErrInvalidFont      // unsupported font directive
    ErrInvalidSchema    // structurally invalid input (e.g. missing header)
    ErrColumnMismatch   // inconsistent column lengths
    ErrParseFailed      // malformed JSON
    ErrInvalidType      // unsupported data type
    ErrInvalidFontSize  // size out of range
    ErrEmptyReport      // no columns / no rows
    ErrNilReport        // nil model
    ErrUnsupportedFormat
    ErrRenderFailed
)
```

---

## 8. Export Workflow

```
Export(report, format)
  └─ Validate(report)                        → *ValidationError on failure
       └─ NewRendererWithOptions(format, opts)
            ├─ validateOptions(opts)
            ├─ mergeOptions(opts)            → fill defaults
            └─ registry[format](opts)        → Renderer
                 └─ renderer.Render(report)  → []byte
                                              (RenderToWriter for streaming paths)

ExportToWriter(report, format, w)
  └─ Validate(report)
       └─ NewRenderer(format)
            ├─ if r implements WriterRenderer  → r.RenderToWriter(report, w)  // streaming
            └─ else → r.Render(report); w.Write(bytes)
```

Renderers run a cheap structural guard (`checkConsistent`) so direct renderer usage never
indexes out of bounds, while full semantic validation is centralized in the Export API.

---

## 9. Backend Integration Examples

All examples assume:

```go
import "github.com/Khaliiloo/reporter"
```

### 9.1 Gin

```go
func DownloadExcel(c *gin.Context) {
    report, err := reports.Parse(c.PostForm("payload")) // or c.Request.Body
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    file, err := reports.Export(report, reports.FormatExcel)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.Data(http.StatusOK,
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file)
}
```

### 9.2 Fiber

```go
func DownloadPDF(c *fiber.Ctx) error {
    report, err := reports.Parse(c.Body())
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
    }
    file, err := reports.Export(report, reports.FormatPDF)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
    }
    c.Set(fiber.HeaderContentType, reports.MimeType(reports.FormatPDF))
    c.Set(fiber.HeaderContentDisposition, `attachment; filename="report.pdf"`)
    return c.Send(file)
}
```

### 9.3 Echo

```go
func DownloadCSV(c echo.Context) error {
    report, err := reports.ParseReader(c.Request().Body)
    if err != nil {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
    }
    c.Response().Header().Set(echo.HeaderContentType, reports.MimeType(reports.FormatCSV))
    return reports.ExportToWriter(report, reports.FormatCSV, c.Response()) // streams
}
```

### 9.4 gRPC service

```go
func (s *reportSvc) Export(_ context.Context, req *pb.ExportRequest) (*pb.ExportReply, error) {
    report, err := reports.Parse(req.GetDefinition())
    if err != nil {
        return nil, status.Errorf(codes.InvalidArgument, "invalid definition: %v", err)
    }
    data, err := reports.ExportWithOptions(report, reports.ExportOptions{
        Format:          reports.FormatExcel,
        SheetName:       req.GetSheetName(),
        FreezeHeader:    true,
        AutoSizeColumns: true,
    })
    if err != nil {
        return nil, status.Errorf(codes.Internal, "render failed: %v", err)
    }
    return &pb.ExportReply{Content: data, MimeType: reports.MimeType(reports.FormatExcel)}, nil
}
```

### 9.5 Background worker

```go
func (w *worker) generate(ctx context.Context, job Job) error {
    report, err := reports.Parse(job.Definition)
    if err != nil {
        return fmt.Errorf("parse: %w", err)
    }
    // Stream straight into object storage — never touches disk.
    obj, err := w.bucket.NewWriter(ctx, job.Key+reports.FileExtension(job.Format), nil)
    if err != nil {
        return err
    }
    defer obj.Close()
    return reports.ExportToWriter(report, job.Format, obj)
}
```

### 9.6 AWS Lambda

```go
func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    report, err := reports.Parse([]byte(req.Body))
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 400, Body: err.Error(),
        }, nil
    }
    data, err := reports.Export(report, reports.FormatWord)
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 500, Body: err.Error(),
        }, nil
    }
    return events.APIGatewayProxyResponse{
        StatusCode: 200,
        Headers: map[string]string{
            "Content-Type": reports.MimeType(reports.FormatWord),
            "Content-Disposition": `attachment; filename="report.docx"`,
        },
        Body:            base64.StdEncoding.EncodeToString(data),
        IsBase64Encoded: true,
    }, nil
}
```

### 9.7 Email attachment

```go
data, _ := reports.Export(report, reports.FormatPDF)
msg.Attach(bytes.NewReader(data), "report.pdf", "application/pdf")
```

---

## 10. Recommended Third-Party Go Libraries

| Purpose | Library used | Rationale / alternatives |
|---|---|---|
| Excel (.xlsx) | `github.com/xuri/excelize/v2` v2.11 | De-facto standard; streaming writer, styles, freeze panes, auto-width. Alternatives: `qax-os/excelize`, `tealeg/xlsx` (maintenance risk). |
| PDF | `github.com/raceresult/gopdf` v1.0.117 | Composite-font PDF engine with embedded TTF support and built-in `pdftext/arabic` contextual shaping. The renderer uses its TextElement pipeline for UTF-8 and Arabic glyph shaping. |
| Word (.docx) | none (hand-rolled OOXML) | License-clean OOXML generation avoids AGPL `unidoc/unioffice`; template-only `nguyenthenguyen/docx` cannot build tables from scratch. Hand-rolled writer keeps deps light and is fully testable. |
| CSV | stdlib `encoding/csv` | RFC 4180, streaming, zero dependencies. |
| Fonts | embedded DejaVu Sans Condensed | Bitstream Vera license, Unicode Latin/Cyrillic/Greek. |

All dependencies are pinned in `go.mod`/`go.sum` (committed); no `go mod tidy` needed
routinely.

---

## 11. Error Handling Strategy

- **Sentinel errors** (`errors.Err*`) classify failures; every returned error wraps one
  via stdlib `%w`, so `errors.Is`/`errors.As` work end-to-end.
- **Structured errors** carry context:
  - `*ValidationError{Kind, Field, Detail}` — field locates the input, e.g.
    `data[2].column_data[1].cell_color`.
  - `*RenderError{Format, Err}` — rendering-phase failure.
  - `*FormatError{Format, Err}` — format-specific failure wrapped by `RenderError`.
- **Wrapping chain example**:

```go
err := reports.Export(bad, reports.FormatExcel)
// errors.Is(err, ErrColumnMismatch)   → true
// errors.As(err, *ValidationError)    → Field "data[1]"
```

- Parsing/rendering errors keep the underlying cause (`invalid JSON: ...`,
  `excel renderer: ...`) via nested `%w`.
- Context-aware (request ID, format, column index) is preserved in `Field`/`Detail`;
  consumers may wrap again with their own request context.

---

## 12. Testing Strategy

| Level | What | Where |
|---|---|---|
| Unit — parsing | valid JSON, type inference, number/string/bool coercion, malformed JSON, invalid type directives | `parser_test.go` |
| Unit — validation | nil/empty reports, missing headers, column mismatch, invalid color/font/size/type, option validation, hex color regex | `validator_test.go` |
| Unit — renderers | signature checks (`%PDF`, `PK`), CSV exact bytes, docx structure, landscape, header/footer parts, XML well-formedness of every generated part | `renderers/renderers_test.go` |
| Integration — Excel | render → reopen with excelize → assert typed cells (numeric, boolean, string), sheet name, freeze panes, auto-width on/off | `TestExcelRoundTrip` |
| Integration — Word | unzip, parse every XML part with `encoding/xml`, assert title/data/header/footer/`PAGE` field | `TestWordXMLWellFormed` |
| Factory/mocks | `mockRenderer` implements the contract; `RegisterRenderer` extends the registry; duplicate registration rejected | `exporter_test.go` |
| API | `ExportToBytes` alias, `ExportToWriter` streaming equality, `SupportedFormats`, MIME/extension | `exporter_test.go` |
| Benchmarks | `Parse`, `Validate`, per-format `Export`, CSV stream-to-discard | `benchmarks_test.go` |

**Mock renderer example**

```go
type mockRenderer struct{ marker string }

func (m *mockRenderer) Render(report *models.Report) ([]byte, error) {
    return []byte(m.marker + ":" + report.Title), nil
}

reports.RegisterRenderer("markdown", func(models.ExportOptions) (reports.Renderer, error) {
    return &mockRenderer{marker: "MD"}, nil
})
```

**Load testing approach.** The streaming path (`ExportToWriter`, `RenderToWriter`) keeps
memory flat for CSV and Excel, so load tests can:
1. Generate an N-row report (or stream rows from a database cursor).
2. Measure peak RSS (`runtime.ReadMemStats`) over `N ∈ {10³, 10⁵, 10⁶}`.
3. Use `io.Discard` to isolate render cost from I/O.
4. For HTTP endpoints, run `hey`/`k6` with concurrent streams and assert P95 latency and
   that OOM is never hit.

---

## 13. Performance Optimization Strategy

- **Streaming output**: `CSVRenderer` and `ExcelRenderer` implement `WriterRenderer`;
  the row transposition uses `RowIterator` which reuses a single row buffer and never
  materializes the full row matrix. PDF/Word buffer in memory by nature of the formats.
- **Streaming input**: `ParseReader` decodes incrementally; cell slices are
  pre-allocated (`make([]Cell, 0, len(...))`).
- **Minimal allocations**: the CSV `record` slice is reused per row; Excel row cells are
  reused; PDF column widths computed once.
- **Style memoization**: the Excel renderer caches style IDs in a map keyed by resolved
  visual properties, so a million-row report yields a tiny stylesheet instead of one
  style per cell.
- **Optional chunk processing**: `RowIterator` + `RenderToWriter` enable chunked
  pipelines (batch of rows → write → flush); consumers can feed rows from a DB cursor.
- **Concurrent rendering when safe**: renderers are immutable; export different reports
  (or the same report in different formats) across goroutines freely.

Measured on this machine (`-benchtime 1x`, 5 cols × 1,000 rows unless noted):

```
BenchmarkParse-20                688 µs/op     (500 cells)
BenchmarkValidate-20            1.20 ms/op     (5,000 cells)
BenchmarkExportCSV-20           1.35 ms/op
BenchmarkExportExcel-20         8.47 ms/op
BenchmarkExportPDF-20          21.8 ms/op      (200 rows)
BenchmarkExportWord-20          2.54 ms/op     (200 rows)
BenchmarkCSVStreamToDiscard-20  1.27 ms/op
```

Run with `go test -bench . -benchmem ./`.

---

## 14. Future Extensibility Plan

1. **New formats** — implement `interfaces.Renderer` (and optionally
   `WriterRenderer`), call `RegisterRenderer`:
   HTML, Markdown, PowerPoint, XML, JSON, images (PNG/JPEG table snapshots).
2. **New data types** — extend `DataType` + coercion in `parser.go` + validator;
   renderers switch on the type (e.g. `date` → native date cells).
3. **Multiple sheets** — extend `ExportOptions` with `[]Sheet` and let `ExcelRenderer`
   emit one sheet per table (the model and renderer interface already support it).
4. **Arabic / complex-script PDF** — the production PDF path uses `raceresult/gopdf`
   composite fonts and its `pdftext/arabic` shaping pipeline. The public report and
   direction APIs remain unchanged, so a future HarfBuzz upgrade can remain renderer-local.
5. **Chunked/batched row ingestion** — add a `RowSource` interface so renderers can
   consume a database cursor instead of an in-memory `Report`.
6. **Options plumbing** — extend `ExportOptions` (page size, margins, table theme) with
   backwards-compatible zero-value defaults.
7. **Content-Disposition / filename helpers** — `Filename(format, title)` for HTTP.

---

## 15. Sample Implementation Skeleton

The complete, tested implementation lives in this module. The essential skeleton:

```go
// models/report.go
type Report struct {
    Title   string
    Header  Header
    Footer  Footer
    Columns []Column
}

// interfaces/renderer.go
type Renderer interface {
    Render(report *models.Report) ([]byte, error)
}

// exporter.go — factory + registry
func NewRenderer(format models.Format) (interfaces.Renderer, error) {
    return NewRendererWithOptions(format, defaultOptionsFor(format))
}

func Export(report *models.Report, format models.Format) ([]byte, error) {
    return ExportWithOptions(report, defaultOptionsFor(format))
}

func ExportWithOptions(report *models.Report, opts models.ExportOptions) ([]byte, error) {
    if err := Validate(report); err != nil {
        return nil, err
    }
    r, err := NewRendererWithOptions(opts.Format, opts)
    if err != nil {
        return nil, err
    }
    return r.Render(report)
}

// renderers/csv.go — minimal renderer
type CSVRenderer struct{ opts models.ExportOptions }

func (r *CSVRenderer) Render(report *models.Report) ([]byte, error) {
    var buf bytes.Buffer
    if err := r.RenderToWriter(report, &buf); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}
```

## 16. Rendering Internals

### 16.1 PDF table layout engine

The PDF renderer implements a deterministic grid-based layout engine. It never
uses fpdf `MultiCell` in a loop (which advances the cursor to the next line and
causes staggered headers/misaligned cells). Instead:

1. **Compute the grid once** — `computeColumnWidths` assigns every column a fixed
   width (proportional to content or equal) that always fits the usable page width;
   `colStarts` records each column's X origin.
2. **Draw at fixed coordinates** — every cell is drawn with explicit
   `(x, y, w, h)` from the shared grid; `SetY` is always called before `SetX`
   (fpdf's `SetY` resets X to the left margin).
3. **Compute row heights first** — `rowHeight` wraps every cell (UTF-8 safe, rune
   aware, metrics-based via `GetStringWidth`) and uses the tallest cell, so rows
   never overlap.
4. **Header in a single row** — all headers share one Y; the header row is
   re-drawn after page breaks.
5. **Document body order** — title (centered) → header_left/header_right metadata
   line → separator rule → table → footer area (page footer). Page-header callbacks
   are not used.

A `pdfLayoutHook` (test-only) records every drawn element so regression tests
assert geometry: header Y equality, row non-overlap, column contiguity, and body
ordering.

### 16.2 DOCX OOXML structure rules

The Word renderer emits strict OOXML. The critical invariant: `w:tbl` is a
**block-level** element and must be a **direct child** of `w:body`, `w:hdr` or
`w:ftr` — never nested inside a `w:p` (a past bug wrapped the header/footer tables
in a paragraph, which Word reports as unreadable content). Regression tests
(`word_test.go`) enforce:

- `w:tbl` is never inside `w:p`;
- header/footer tables are the direct first child of `w:hdr`/`w:ftr`;
- document body order is title → metadata (tab stop) → separator (`w:pBdr`) → table;
- every relationship target exists in the package and every part is declared in
  `[Content_Types].xml`.

A hand-rolled OOXML writer is used deliberately: `unioffice` is AGPL, `godocx`
(v0.1.x) lacks table borders and header/footer creation, and template-only DOCX
libraries cannot build tables from scratch. The package structure is validated
structurally instead (see §12).

---

## RTL and internationalization

`text_direction` is optional JSON and defaults to `LTR`. In `RTL` mode, renderers create
a reversed view of the columns and each row while leaving the source model unchanged.
Numeric cells remain right-aligned; text cells use right alignment. Header/footer metadata
retains its original left/right meaning and is never swapped. DOCX uses native
`w:bidiVisual`, `w:bidi`, and `w:rtl` properties. PDF uses `golang.org/x/text/unicode/bidi`
to order directional runs before drawing. Parser defaulting, column reversal, and model
immutability are covered by the RTL regression tests.

## Known limitations

- **PDF complex scripts**: the selected gopdf Arabic shaper covers Arabic joining forms;
  broader script coverage may require a HarfBuzz-backed engine.
- **Go version**: the module requires **Go ≥ 1.25** (excelize v2.11 dependency).
- **Date cells** are exported as text; native Excel date cells require an explicit date
  parse + number format (extension point, see §14.2).
- **PDF/Word memory**: these formats are buffered in memory by design; for huge exports
  prefer CSV/Excel streaming.
- **PDF metadata**: fpdf embeds a per-render `CreationDate` (second granularity), so PDF
  bytes are not byte-deterministic across seconds. Call
  `renderer.(*interfaces.Renderer)` consumers that need reproducible PDFs can set a fixed
  creation date via the fpdf layer in a custom renderer.
