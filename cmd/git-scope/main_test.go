package main

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCmd string
		wantArg []string
	}{
		{name: "empty args", args: nil, wantCmd: "", wantArg: nil},
		{name: "scan command", args: []string{"scan", "."}, wantCmd: "scan", wantArg: []string{"."}},
		{name: "help command", args: []string{"help"}, wantCmd: "help", wantArg: []string{}},
		{name: "unknown defaults to tui", args: []string{"/tmp"}, wantCmd: "tui", wantArg: []string{"/tmp"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCmd, gotArg := parseCommand(tc.args)
			if gotCmd != tc.wantCmd {
				t.Fatalf("cmd mismatch: got %q want %q", gotCmd, tc.wantCmd)
			}
			if len(gotArg) != len(tc.wantArg) {
				t.Fatalf("arg len mismatch: got %d want %d", len(gotArg), len(tc.wantArg))
			}
			for i := range gotArg {
				if gotArg[i] != tc.wantArg[i] {
					t.Fatalf("arg[%d] mismatch: got %q want %q", i, gotArg[i], tc.wantArg[i])
				}
			}
		})
	}
}
