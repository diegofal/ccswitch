package cli

import (
	"fmt"
	"io"
)

// out is a writer that keeps its first error: printing goes on without
// checks at every line, and Run turns a failed write (a closed pipe) into
// exit 1 instead of losing it.
type out struct {
	w   io.Writer
	err error
}

func (o *out) printf(format string, a ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprintf(o.w, format, a...)
	}
}

func (o *out) println(a ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprintln(o.w, a...)
	}
}
