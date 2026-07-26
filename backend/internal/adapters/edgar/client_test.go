package edgar

import "testing"

func TestClassify(t *testing.T) {
	c := NewClient("test")
	cases := []struct {
		name       string
		acceptance string
		want       string
	}{
		// 20:02 UTC = 16:02 EDT (summer) -> after 16:00 close -> amc
		{"after close (EDT)", "2026-06-24T20:02:01.000Z", "amc"},
		// 21:03 UTC = 16:03 EST (winter) -> amc
		{"after close (EST)", "2025-12-17T21:03:26.000Z", "amc"},
		// 12:00 UTC = 08:00 EDT -> before 09:30 open -> bmo
		{"before open (EDT)", "2026-05-01T12:00:00.000Z", "bmo"},
		// 17:00 UTC = 13:00 EDT -> mid-session -> unknown
		{"mid session", "2026-05-01T17:00:00.000Z", ""},
		{"unparseable", "not-a-date", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.classify(tc.acceptance); got != tc.want {
				t.Fatalf("classify(%s) = %q, want %q", tc.acceptance, got, tc.want)
			}
		})
	}
}
