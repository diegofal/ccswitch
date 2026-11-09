package execx

import "testing"

func TestStripANSI(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text stays as it is", "ok 12 tests\n  passed", "ok 12 tests\n  passed"},
		{"empty", "", ""},
		{
			"vitest's missing dependency banner",
			"\x1b[31m\x1b[7m\x1b[31m MISSING DEPENDENCY \x1b[31m\x1b[27m Cannot find dependency '@vitest/coverage-v8'\x1b[39m",
			" MISSING DEPENDENCY  Cannot find dependency '@vitest/coverage-v8'",
		},
		{"colours around UTF-8", "\x1b[1;32m✓\x1b[0m año ünïcode 日本", "✓ año ünïcode 日本"},
		{"cursor and private modes", "\x1b[2K\x1b[1G\x1b[?25lrunning\x1b[?25h", "running"},
		{"OSC hyperlink ended by ST", "see \x1b]8;;https://vitest.dev/guide\x1b\\the guide\x1b]8;;\x1b\\.", "see the guide."},
		{"OSC title ended by BEL", "\x1b]0;vitest\x07done", "done"},
		{"unterminated OSC stops at the line end", "\x1b]8;;https://x\nnext line", "\nnext line"},
		{"two-byte and charset sequences", "\x1b7a\x1b8\x1b(Bb\x1b=c", "abc"},
		{"lone ESC at the end", "end\x1b", "end"},
		{"CSI cut short at the end", "end\x1b[31", "end"},
		{"CSI broken by a line end keeps the line end", "a\x1b[31\nb", "a\nb"},
		{"ESC before a non-ASCII byte drops only the ESC", "\x1bé", "é"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripANSI(c.in); got != c.want {
				t.Errorf("stripANSI(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
