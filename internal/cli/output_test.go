package cli

import "testing"

func TestCountLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 1},
		{"single line", "hello", 1},
		{"two lines", "a\nb", 2},
		{"trailing newline", "a\nb\n", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := countLines(tc.in); got != tc.want {
				t.Errorf("countLines(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty returns nil", "", nil},
		{"single line", "hello", []string{"hello"}},
		{"two lines", "a\nb", []string{"a", "b"}},
		{"trailing newline stripped", "a\nb\n", []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitLines(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitLines(%q) = %v (len %d), want %v (len %d)", tc.in, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}
