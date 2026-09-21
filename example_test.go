package reports_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	reports "github.com/Khaliiloo/reporter"
)

// Example_httpHandler demonstrates embedding the library in a plain net/http
// handler: parse a JSON definition, render to Excel and stream the bytes to
// the client.
func Example_httpHandler() {
	var jsonData = []byte(`{
		"title": "Monthly Flights",
		"header_left": "Rehlaa",
		"data": [
			{"column_header": "Route", "column_data": [
				{"value": "CAI-DXB"}, {"value": "JED-LHR"}
			]},
			{"column_header": "Passengers", "column_data": [
				{"value": 142}, {"value": 98}
			]}
		]
	}`)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		report, err := reports.Parse(jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", reports.MimeType(reports.FormatExcel))
		w.Header().Set("Content-Disposition", `attachment; filename="flights.xlsx"`)
		if err := reports.ExportToWriter(report, reports.FormatExcel, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/reports/flights.xlsx", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	fmt.Printf("status=%d content-type=%s\n",
		rec.Code, rec.Header().Get("Content-Type"))
	// Output: status=200 content-type=application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
}
