package main

import (
	"fmt"
	"os"

	reports "github.com/Khaliiloo/reporter"
)

const jsonData = `{
	"title": "My First Report",
	"header_left": "Header Left",
	"header_right": "Header Right",
	"footer_left": "Footer Left",
	"footer_right": "Footer Right",
	"data": [
		{
			"column_header": "Name",
			"style": {"header_font_style": "bold", "header_font_size": 12, "data_font_size": 11},
			"column_data": [
				{"value": "John", "type": "string"},
				{"value": "Jane", "type": "string"},
				{"value": "Mohamed عبد الله", "type": "string"}
			]
		},
		{
			"column_header": "Score",
			"style": {"data_font_style": "italic", "data_font_color": "#333333"},
			"column_data": [
				{"value": 95.5, "type": "number"},
				{"value": 88, "type": "number"},
				{"value": 72.25, "type": "number"}
			]
		},
		{
			"column_header": "Total",
			"column_data": [
				{"value": 75, "type": "number"},
				{"value": 100, "type": "number"},
				{"value": "A long description that wraps across multiple lines to demonstrate word wrapping", "type": "string"}
			]
		}
	]
}`

func main() {
	report, err := reports.Parse([]byte(jsonData))
	if err != nil {
		panic(err)
	}
	if err := reports.Validate(report); err != nil {
		panic(err)
	}

	formats := []reports.Format{reports.FormatExcel, reports.FormatCSV, reports.FormatPDF, reports.FormatWord}
	for _, f := range formats {
		data, err := reports.Export(report, f)
		if err != nil {
			panic(err)
		}
		name := "sample" + reports.FileExtension(f)
		if err := os.WriteFile(name, data, 0o644); err != nil {
			panic(err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", name, len(data))
	}
}
