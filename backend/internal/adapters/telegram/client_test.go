package telegram

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantChat    int64
		wantCommand string
		wantArgs    string
		wantOK      bool
	}{
		{
			name:        "simple command with args",
			body:        `{"message":{"chat":{"id":42},"text":"/reports MU"}}`,
			wantChat:    42,
			wantCommand: "reports",
			wantArgs:    "MU",
			wantOK:      true,
		},
		{
			name:        "command with botname suffix in group",
			body:        `{"message":{"chat":{"id":-100},"text":"/reports@fin_bot 2 AAPL"}}`,
			wantChat:    -100,
			wantCommand: "reports",
			wantArgs:    "2 AAPL",
			wantOK:      true,
		},
		{
			name:        "bare command no args",
			body:        `{"message":{"chat":{"id":7},"text":"/start"}}`,
			wantChat:    7,
			wantCommand: "start",
			wantArgs:    "",
			wantOK:      true,
		},
		{"plain text is not a command", `{"message":{"chat":{"id":1},"text":"hello"}}`, 0, "", "", false},
		{"no message (e.g. edited/callback)", `{"update_id":1}`, 0, "", "", false},
		{"malformed json", `not json`, 0, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, cmd, args, ok := ParseCommand([]byte(tc.body))
			if ok != tc.wantOK || chat != tc.wantChat || cmd != tc.wantCommand || args != tc.wantArgs {
				t.Fatalf("ParseCommand = (%d, %q, %q, %v), want (%d, %q, %q, %v)",
					chat, cmd, args, ok, tc.wantChat, tc.wantCommand, tc.wantArgs, tc.wantOK)
			}
		})
	}
}
