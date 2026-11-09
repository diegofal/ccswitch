package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseVersion reads "2.1.300" or `claude --version`'s "2.1.300 (Claude Code)".
func ParseVersion(s string) ([3]int, error) {
	var v [3]int
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return v, fmt.Errorf("version %q: empty", s)
	}
	parts := strings.Split(fields[0], ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("version %q: want major.minor.patch", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return v, fmt.Errorf("version %q: %q is not a number", s, p)
		}
		v[i] = n
	}
	return v, nil
}

// CompareVersion returns -1, 0 or 1 as a is older than, equal to or newer
// than b.
func CompareVersion(a, b string) (int, error) {
	va, err := ParseVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := ParseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := range va {
		switch {
		case va[i] < vb[i]:
			return -1, nil
		case va[i] > vb[i]:
			return 1, nil
		}
	}
	return 0, nil
}
