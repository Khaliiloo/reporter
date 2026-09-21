package reports

import (
	"sort"
	"sync"

	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/interfaces"
	"github.com/Khaliiloo/reporter/models"
	"github.com/Khaliiloo/reporter/renderers"
)

// RendererFactory constructs a renderer for a format with resolved options.
// Factories are registered per format and must be safe to call concurrently.
type RendererFactory func(opts models.ExportOptions) (interfaces.Renderer, error)

var (
	rendererRegistryMu sync.RWMutex
	rendererRegistry   = map[models.Format]RendererFactory{
		models.FormatExcel: func(opts models.ExportOptions) (interfaces.Renderer, error) {
			return renderers.NewExcelRenderer(opts), nil
		},
		models.FormatCSV: func(opts models.ExportOptions) (interfaces.Renderer, error) {
			return renderers.NewCSVRenderer(opts), nil
		},
		models.FormatPDF: func(opts models.ExportOptions) (interfaces.Renderer, error) {
			return renderers.NewPDFRenderer(opts), nil
		},
		models.FormatWord: func(opts models.ExportOptions) (interfaces.Renderer, error) {
			return renderers.NewWordRenderer(opts), nil
		},
	}
)

// DefaultOptions returns export options with sensible library defaults.
func DefaultOptions() models.ExportOptions {
	return models.ExportOptions{
		SheetName:       "Sheet1",
		PageOrientation: models.OrientationPortrait,
		AutoSizeColumns: true,
		FreezeHeader:    true,
	}
}

func defaultOptionsFor(format models.Format) models.ExportOptions {
	opts := DefaultOptions()
	opts.Format = format
	return opts
}

// RegisterRenderer adds a renderer factory for a new format. It returns an
// error if the format already has a registered factory. This is the
// extension point for adding HTML, Markdown, PowerPoint, XML, JSON, image or
// other renderers without modifying existing code.
func RegisterRenderer(format models.Format, factory RendererFactory) error {
	if factory == nil {
		return errors.NewValidationError(errors.ErrInvalidSchema, "", "renderer factory is nil")
	}
	rendererRegistryMu.Lock()
	defer rendererRegistryMu.Unlock()
	if _, exists := rendererRegistry[format]; exists {
		return errors.NewValidationError(
			errors.ErrUnsupportedFormat, "", "a renderer for format %q is already registered", format)
	}
	rendererRegistry[format] = factory
	return nil
}

// NewRenderer resolves a renderer for format with default options.
func NewRenderer(format models.Format) (interfaces.Renderer, error) {
	return NewRendererWithOptions(format, defaultOptionsFor(format))
}

// NewRendererWithOptions resolves a renderer for opts.Format using the given
// options. Unset option fields receive library defaults.
func NewRendererWithOptions(format models.Format, opts models.ExportOptions) (interfaces.Renderer, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	opts = mergeOptions(opts)

	rendererRegistryMu.RLock()
	factory, ok := rendererRegistry[format]
	rendererRegistryMu.RUnlock()
	if !ok {
		return nil, errors.NewValidationError(
			errors.ErrUnsupportedFormat, "", "no renderer registered for format %q", format)
	}
	return factory(opts)
}

// mergeOptions fills unset option fields with defaults.
func mergeOptions(opts models.ExportOptions) models.ExportOptions {
	d := DefaultOptions()
	if opts.SheetName == "" {
		opts.SheetName = d.SheetName
	}
	if opts.PageOrientation == "" {
		opts.PageOrientation = d.PageOrientation
	}
	return opts
}

// Export renders report in the requested format and returns the output bytes.
// It validates the report automatically; calling Validate explicitly is only
// needed when building models programmatically and deferring export.
func Export(report *models.Report, format models.Format) ([]byte, error) {
	return ExportWithOptions(report, defaultOptionsFor(format))
}

// ExportToBytes is an alias for Export, provided for API clarity.
func ExportToBytes(report *models.Report, format models.Format) ([]byte, error) {
	return Export(report, format)
}

// ExportWithOptions renders report using the given options (format, sheet
// name, orientation, auto-sizing, freeze panes).
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

// SupportedFormats returns the formats with registered renderers, sorted.
func SupportedFormats() []models.Format {
	rendererRegistryMu.RLock()
	defer rendererRegistryMu.RUnlock()
	formats := make([]models.Format, 0, len(rendererRegistry))
	for f := range rendererRegistry {
		formats = append(formats, f)
	}
	sort.Slice(formats, func(i, j int) bool { return formats[i] < formats[j] })
	return formats
}

// MimeType returns the standard media type for a format.
func MimeType(format models.Format) string {
	switch format {
	case models.FormatExcel:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case models.FormatCSV:
		return "text/csv; charset=utf-8"
	case models.FormatPDF:
		return "application/pdf"
	case models.FormatWord:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

// FileExtension returns the file extension (with leading dot) for a format.
func FileExtension(format models.Format) string {
	switch format {
	case models.FormatExcel:
		return ".xlsx"
	case models.FormatCSV:
		return ".csv"
	case models.FormatPDF:
		return ".pdf"
	case models.FormatWord:
		return ".docx"
	default:
		return ""
	}
}
