package restart

import "testing"

func TestFindPanic(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"clean", []string{"started server", "serving /allocation"}, false},
		{"panic", []string{"ok", "panic: runtime error: invalid memory address"}, true},
		{"fatal", []string{"FATAL ERROR: out of memory"}, true},
		{"empty", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindPanic(tc.lines)
			if (got != "") != tc.want {
				t.Fatalf("FindPanic(%v) = %q, want hit=%t", tc.lines, got, tc.want)
			}
		})
	}
}
