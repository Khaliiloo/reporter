package renderers

import (
	"github.com/Khaliiloo/reporter/errors"
	"github.com/Khaliiloo/reporter/models"
)

// checkConsistent is a cheap, panic-free structural guard renderers run before
// touching cells. Full semantic validation is the responsibility of the root
// package's Validate / Export APIs.
func checkConsistent(report *models.Report) error {
	if report == nil {
		return errors.NewValidationError(errors.ErrNilReport, "", "report is nil")
	}
	n := -1
	for i := range report.Columns {
		lenCol := len(report.Columns[i].ColumnData)
		if n == -1 {
			n = lenCol
		} else if lenCol != n {
			return errors.NewValidationError(
				errors.ErrColumnMismatch, "",
				"all columns must have the same number of data rows")
		}
	}
	if n == 0 {
		return errors.NewValidationError(errors.ErrEmptyReport, "", "report has no data rows")
	}
	return nil
}

// RowIterator transposes the column-major report model into rows on demand.
// It is used by streaming renderers (CSV, Excel) to avoid materializing the
// entire row matrix in memory.
//
// Row returns a slice that is reused between iterations; callers must copy
// values they intend to keep.
type RowIterator struct {
	report *models.Report
	rowIdx int
	n      int
	buf    []models.Cell
}

// NewRowIterator builds a RowIterator over report. The report must be
// structurally consistent; call checkConsistent first.
func NewRowIterator(report *models.Report) *RowIterator {
	return &RowIterator{
		report: report,
		n:      report.RowCount(),
	}
}

// Next advances to the next row, returning false at the end.
func (it *RowIterator) Next() bool {
	if it.rowIdx >= it.n {
		return false
	}
	it.rowIdx++
	return true
}

// Row returns the current row. The backing array is reused across iterations.
func (it *RowIterator) Row() []models.Cell {
	if it.buf == nil {
		it.buf = make([]models.Cell, len(it.report.Columns))
	}
	for j := range it.report.Columns {
		if len(it.report.Columns[j].ColumnData) > 0 {
			it.buf[j] = it.report.Columns[j].ColumnData[it.rowIdx-1]
		} else {
			it.buf[j] = models.Cell{Value: "", Type: models.DataTypeString}
		}
	}
	return it.buf
}
