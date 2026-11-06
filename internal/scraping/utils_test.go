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

func TestConvertStringToInt(t *testing.T) {
	cases := map[string]int{
		`0" Past 48HRS`:           0,
		`12" Past 24HRS`:          12,
		`18 in Mid-Mountain Base`: 18,
		`Season Total: 7"`:        7,
		`1,234`:                   1234,
		`4.5"`:                    4,
		`--`:                      0,
		``:                        0,
		`no data`:                 0,
	}
	for in, want := range cases {
		got, err := convertStringToInt(in)
		if err != nil || got != want {
			t.Errorf("convertStringToInt(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}
