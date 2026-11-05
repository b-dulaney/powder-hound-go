package scraping

import "testing"

func TestParseTotal(t *testing.T) {
	cases := map[string]int{
		"12 of 147":                  147,
		"12/147":                     147,
		"12 / 147":                   147,
		"Lifts Open: 0 of 9":         9,
		"0 of 56 runs open":          56,
		"Trails Open 1,203 of 2,500": 2500,
		"12":                         0,
		"—":                          0,
		"":                           0,
		"Lifts Open":                 0,
		"12 offered":                 0,
	}
	for in, want := range cases {
		if got := parseTotal(in); got != want {
			t.Errorf("parseTotal(%q) = %d, want %d", in, got, want)
		}
	}
}
