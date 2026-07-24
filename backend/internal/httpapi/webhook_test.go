package httpapi

import "testing"

func TestParseReportsArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       string
		wantYears  int
		wantTicker string
		wantOK     bool
	}{
		{"ticker only defaults to 1 year", "MU", 1, "MU", true},
		{"lowercase ticker uppercased", "mu", 1, "MU", true},
		{"explicit 1 year", "1 AAPL", 1, "AAPL", true},
		{"explicit 2 years", "2 aapl", 2, "AAPL", true},
		{"unknown depth token", "3 AAPL", 0, "", false},
		{"non-numeric first of two", "foo AAPL", 0, "", false},
		{"empty", "", 0, "", false},
		{"too many tokens", "1 2 AAPL", 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			years, ticker, ok := parseReportsArgs(tc.args)
			if ok != tc.wantOK || years != tc.wantYears || ticker != tc.wantTicker {
				t.Fatalf("parseReportsArgs(%q) = (%d, %q, %v), want (%d, %q, %v)",
					tc.args, years, ticker, ok, tc.wantYears, tc.wantTicker, tc.wantOK)
			}
		})
	}
}
