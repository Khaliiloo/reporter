package models

// ExportOptions carries configurable, per-export settings that are resolved at
// renderer construction time. Zero values are replaced by sane defaults via
// reports.DefaultOptions().
type ExportOptions struct {
	// Format selects the output format.
	Format Format

	// SheetName is the worksheet name used by the Excel renderer.
	SheetName string

	// PageOrientation is OrientationPortrait or OrientationLandscape and is
	// used by the PDF and Word renderers.
	PageOrientation string

	// CompressOutput requests a compressed artifact. Currently relevant to
	// formats whose container already compresses (xlsx/docx); renderers that
	// cannot honor it ignore the flag.
	CompressOutput bool

	// AutoSizeColumns requests column width estimation for tabular formats
	// (Excel, PDF, Word).
	AutoSizeColumns bool

	// FreezeHeader freezes the header row for Excel output.
	FreezeHeader bool
}
