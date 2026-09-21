package reports

import "testing"

func TestParseTextDirectionDefaultsToLTR(t *testing.T) {
	r, err := Parse([]byte(`{"title":"Sales","data":[{"column_header":"A","column_data":[{"value":"x","type":"string"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.TextDirection != TextDirectionLTR {
		t.Fatalf("direction = %q", r.TextDirection)
	}
}

func TestParseTextDirectionRTL(t *testing.T) {
	r, err := Parse([]byte(`{"text_direction":"RTL","data":[{"column_header":"اسم","column_data":[{"value":"أحمد","type":"string"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.TextDirection != TextDirectionRTL {
		t.Fatalf("direction = %q", r.TextDirection)
	}
}
