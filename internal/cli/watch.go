package cli

import (
	"time"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
)

// watch is the poll loop without a tray (PLAN D11): one core.Poll every
// PollActive (the core decides which accounts are due), one status line per
// round, until the context ends (Ctrl-C), which is a normal exit. A failed
// round is printed and, since a headless loop has nobody watching its
// terminal, notified, once per distinct error. --once runs one round and
// prints the full status.
func (r *runner) watch(rest []string) int {
	once := false
	switch {
	case len(rest) == 1 && rest[0] == "--once":
		once = true
	case len(rest) != 0:
		return r.usage("watch takes only --once")
	}
	sw := r.d.Switcher
	if once {
		return r.result(sw.Poll(r.ctx, false))
	}
	after := r.d.After
	if after == nil {
		after = time.After
	}
	r.stdout.println("Watching; Ctrl-C stops.")
	notified := ""
	for {
		st, err := sw.Poll(r.ctx, false)
		if r.ctx.Err() != nil {
			return exitOK
		}
		set, serr := sw.Settings()
		if serr != nil {
			set = domain.DefaultSettings()
			if err == nil {
				err = serr
			}
		}
		now := r.d.Clock.Now()
		r.stdout.printf("%s  %s\n", clock(now), core.StatusLine(st, set, now))
		notified = r.roundError(err, notified)
		every := set.PollActive
		if every <= 0 {
			every = domain.DefaultSettings().PollActive
		}
		select {
		case <-r.ctx.Done():
			return exitOK
		case <-after(every):
		}
	}
}

// roundError prints a failed round's error and notifies it unless it is the
// one already notified; it returns the error now notified ("" after a good
// round, so the same failure coming back is said again).
func (r *runner) roundError(err error, notified string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	r.stderr.printf("ccswitch: %s\n", msg)
	if msg == notified {
		return notified
	}
	if nerr := r.d.Notify.Notify("ccswitch", msg); nerr != nil {
		r.stderr.printf("ccswitch: notify: %v\n", nerr)
		return notified
	}
	return msg
}
