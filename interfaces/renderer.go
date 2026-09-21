// Package interfaces defines the renderer contracts. The root reports package
// re-exports these types via aliases so consumers can use either
// reports.Renderer or interfaces.Renderer interchangeably.
package interfaces

import (
	"io"

	"dev.azure.com/CubicLYDev/Rehlaa/_git/reporter.git/models"
)

// Renderer converts an in-memory report model into bytes in a specific output
// format. Implementations must be safe for concurrent use: a single instance
// is returned by the factory and may be shared by multiple goroutines.
//
// Implementations assume the report has been validated (see reports.Validate
// or the high-level Export API, which validate automatically). Renderers must
// still guard against panics on structurally inconsistent input by checking
// column length consistency before indexing cells.
type Renderer interface {
	Render(report *models.Report) ([]byte, error)
}

// WriterRenderer is an optional extension for renderers that can stream their
// output directly to an io.Writer. The high-level reports.ExportToWriter API
// prefers this path when available, enabling memory-efficient exports of very
// large datasets.
type WriterRenderer interface {
	Renderer

	// RenderToWriter writes the rendered report to w. It must not close w.
	RenderToWriter(report *models.Report, w io.Writer) error
}
