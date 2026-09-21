# Repository Guidelines

## Project Structure & Module Organization

This repository is the `github.com/Khaliiloo/reporter` Go module, a framework-agnostic report-export library. The root package provides parsing, validation, renderer registration, and public export APIs (`parser.go`, `validator.go`, `exporter.go`, `report.go`). Domain types and options live in `models/`; renderer contracts live in `interfaces/`; format implementations and their tests live in `renderers/`. Shared error types are in `errors/`. Sample inputs are kept at the root and under `_samples/`.

## Build, Test, and Development Commands

- `go test ./...` runs the complete unit and integration-style test suite.
- `go test -race ./...` checks the registry and renderers for data races (not runnable on the default Windows checkout; see Known Gotchas).
- `go test -bench . -benchmem ./` runs export and parsing benchmarks with allocation data.
- `go vet ./...` performs standard Go static checks.
- `gofmt -w .` formats Go files before committing; limit the command to changed files when practical (all files are CRLF; see Known Gotchas).

The module targets Go 1.25. Run commands from the repository root.

## Coding Style & Naming Conventions

Use idiomatic Go and `gofmt` formatting (tabs for indentation). Keep exported identifiers documented and use clear PascalCase names such as `ExportWithOptions`; use camelCase for unexported names. Format implementations belong in `renderers/<format>.go` and must depend on `models` and `interfaces`, not the root package, to avoid import cycles. Prefer wrapped errors (`fmt.Errorf("context: %w", err)`) so callers can use `errors.Is` and `errors.As`.

## Testing Guidelines

Place root-package tests in `*_test.go`; keep renderer tests under `renderers/`. Name tests `Test<Behavior>` and benchmarks `Benchmark<Operation>`, for example `TestExcelRoundTrip`. Cover success and invalid-input paths. For generated files, assert structural or round-trip properties rather than brittle byte equality (PDF output includes creation metadata).

`renderers/renderers_test.go` is the umbrella test file: add tests for shared helpers (`rows.go`, `text.go`, `image.go`) and cross-format behavior there instead of creating per-file test files. Per-format files exist only for `pdf`, `word`, `logo`, and `rtl`. Reuse the `onePixelPNG` base64 constant from `logo_test.go` for image tests instead of embedding new fixtures.

## Known Gotchas

Environment and tooling traps confirmed on this checkout — check here before debugging a surprising failure:

- **`gofmt -l .` flags every file.** The whole repo uses CRLF line endings, which gofmt reports as needing formatting. Confirm with `gofmt -d <file>` that the only diff is line endings before acting, and never run `gofmt -w .` repo-wide from Windows — it rewrites line endings on every file and drowns the real diff.
- **`go test -race` cannot run locally.** The race detector requires cgo (`CGO_ENABLED=1`) and a C compiler; the Windows checkout has no `gcc` in PATH, so the build fails before any test runs. Use plain `go test ./...`; run `-race` only from an environment with a C toolchain (e.g. CI).
- **`encoding/csv` does not strip the UTF-8 BOM.** When asserting CSV output that sets `CSVOptions.IncludeBOM`, compare the bytes after the 3-byte `\xEF\xBB\xBF` prefix (e.g. against plain-renderer output) rather than re-parsing, because the parsed first field will retain the BOM rune.
- **`models.Cell.String()` formats numbers, it does not print them verbatim.** It uses `%.3f` and switches to `K`/`M`/`B` suffixes at 1,000 / 1,000,000 / 1,000,000,000 — `90.5` becomes `"90.500"` in CSV output. Write value assertions against the formatted form.
- **`renderers`' `checkConsistent` deliberately returns `nil` for a zero-column report.** It is only a row-consistency guard; schema-level validation (missing columns, empty report) belongs to the root package's `Validate`. Do not assert `ErrEmptyReport` for zero columns in renderer tests.

## Commit & Pull Request Guidelines

Commit history was not available from this module checkout; use short, imperative subjects scoped to the change, for example `Add CSV header validation`. Keep commits focused. Pull requests should explain the user-facing change, identify affected formats, list validation commands run, link relevant issues, and include a small sample output or screenshot when visual PDF, Excel, or Word layout changes.

## Security & Configuration

The library returns bytes or writes to a caller-provided writer; do not add implicit disk writes or embed credentials. Treat untrusted report definitions as input to parse and validate before rendering. Preserve the public API and renderer registry’s concurrency safety when extending formats.
