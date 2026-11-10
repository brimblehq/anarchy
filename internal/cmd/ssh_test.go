package cmd

import "testing"

func TestProcessStdin(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantOut    string
		wantDetach bool
	}{
		{"plain input", "echo hi\r", "echo hi\r", false},
		{"detach", "~.", "", true},
		{"detach after a command", "ls\r~.", "ls\r", true},
		{"literal tilde", "~~", "~", false},
		{"tilde mid-line passes through", "ls ~/src\r", "ls ~/src\r", false},
		{"unknown escape passes through", "~x", "~x", false},
		{"tilde then enter passes through", "~\r", "~\r", false},
		{"tilde not at line start", "echo ~\r", "echo ~\r", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := stdinEscapeState{atLineStart: true}
			out, detach := processStdin([]byte(tt.in), &st)
			if string(out) != tt.wantOut {
				t.Fatalf("out = %q, want %q", out, tt.wantOut)
			}
			if detach != tt.wantDetach {
				t.Fatalf("detach = %v, want %v", detach, tt.wantDetach)
			}
		})
	}
}

func TestProcessStdinAcrossReads(t *testing.T) {
	st := stdinEscapeState{atLineStart: true}

	out, detach := processStdin([]byte("echo hi\r~"), &st)
	if string(out) != "echo hi\r" || detach {
		t.Fatalf("after '~', out = %q, detach = %v", out, detach)
	}

	out, detach = processStdin([]byte("."), &st)
	if string(out) != "" || !detach {
		t.Fatalf("after '.', out = %q, detach = %v", out, detach)
	}
}
