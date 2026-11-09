// Package shell is the tray app's Wails glue: it draws the rows, icon and
// tooltip internal/tray renders from the state, and turns clicks into core
// calls. It holds no logic of its own (CLAUDE.md rule 6) and is tested by
// running the app on a test copy.
package shell

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/tray"
)

const (
	// watchEvery is how often the tray looks for changes made outside it:
	// the live login (a CLI switch, Claude Code refreshing its token) and
	// state.json (any CLI command), PLAN D12.
	watchEvery = 10 * time.Second
	// opTimeout bounds one core call, waiting for the lock included.
	opTimeout = 5 * time.Minute
	// updateTimeout bounds `claude update`, which downloads a release.
	updateTimeout = 15 * time.Minute
)

// Options are what the wiring gives the tray.
type Options struct {
	Switcher *core.Switcher
	Notifier *Notifier // also the Switcher's ports.Notifier
	// LoginModTime is the live login's modification time
	// (ports.LoginStore.ModTime).
	LoginModTime func() (time.Time, error)
	// Start starts a program and does not wait for it (through execx).
	Start   func(name string, args ...string) error
	CLIPath string // the ccswitch CLI beside the app, run by "Add account…"
	LogPath string // ccswitch.log, opened by "Open log"
	Logf    func(format string, args ...any)
	Version string
}

type shell struct {
	o        Options
	ctx      context.Context
	app      *application.App
	tray     *application.SystemTray
	menu     *application.Menu
	mu       sync.Mutex // one render at a time
	updating atomic.Bool
	drawnAt  time.Time // UpdatedAt of the state last drawn
}

// Run starts the tray app and blocks until the user quits or ctx ends.
func Run(ctx context.Context, o Options) error {
	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := &shell{o: o, ctx: appCtx}
	s.app = application.New(application.Options{
		Name:        "ccswitch",
		Description: "Claude Code accounts · " + o.Version,
		Icon:        tray.IconPNG(75, tray.LevelOK),
		Services:    []application.Service{application.NewService(o.Notifier.ns)},
		Mac:         application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
	})
	s.menu = s.app.NewMenu()
	s.tray = s.app.SystemTray.New()
	s.tray.SetMenu(s.menu)
	// Wails opens the menu on a right click only; the tray has nothing
	// else to show, so a left click opens it too.
	s.tray.OnClick(s.tray.OpenMenu)
	s.tray.SetIcon(tray.IconPNG(0, tray.LevelStale)) // until the first draw
	s.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go s.loop()
	})
	stop := context.AfterFunc(ctx, s.app.Quit)
	defer stop()
	if err := s.app.Run(); err != nil {
		return fmt.Errorf("tray: %w", err)
	}
	return nil
}

// loop polls on the settings' active interval and watches for changes
// made outside the tray, until the app ends.
func (s *shell) loop() {
	s.redraw()
	s.op("poll", opTimeout, func(ctx context.Context) (domain.State, error) { return s.o.Switcher.Poll(ctx, true) })
	set, err := s.o.Switcher.Settings()
	if err != nil {
		s.o.Logf("tray: %v; polling every %s", err, domain.DefaultSettings().PollActive)
		set = domain.DefaultSettings()
	}
	poll := time.NewTicker(set.PollActive)
	defer poll.Stop()
	watch := time.NewTicker(watchEvery)
	defer watch.Stop()
	lastMod, _ := s.o.LoginModTime()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-poll.C:
			s.op("poll", opTimeout, func(ctx context.Context) (domain.State, error) { return s.o.Switcher.Poll(ctx, false) })
		case <-watch.C:
			mod, err := s.o.LoginModTime()
			if err == nil && !mod.Equal(lastMod) {
				lastMod = mod
				s.op("capture", opTimeout, s.o.Switcher.Capture)
				continue
			}
			s.redrawIfChanged()
		}
	}
}

// op runs one core call and draws the state it returns. The core has
// already saved its error into State.LastError, which the menu shows; the
// log gets it too.
func (s *shell) op(name string, timeout time.Duration, fn func(context.Context) (domain.State, error)) {
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	st, err := fn(ctx)
	if err != nil {
		s.o.Logf("tray: %s: %v", name, err)
	}
	s.draw(st)
}

// redraw draws the saved state. Drawing needs the app running.
func (s *shell) redraw() {
	st, err := s.o.Switcher.State(s.ctx)
	if err != nil {
		st.LastError = err.Error()
	}
	s.draw(st)
}

// redrawIfChanged draws the saved state when another process (the CLI)
// has changed it since the last draw.
func (s *shell) redrawIfChanged() {
	st, err := s.o.Switcher.State(s.ctx)
	s.mu.Lock()
	same := err == nil && st.UpdatedAt.Equal(s.drawnAt)
	s.mu.Unlock()
	if !same {
		s.redraw()
	}
}

func (s *shell) draw(st domain.State) {
	set, err := s.o.Switcher.Settings()
	if err != nil {
		set = domain.DefaultSettings()
		st.LastError = joinErr(st.LastError, err.Error())
	}
	ui := tray.UI{Updating: s.updating.Load()}
	if on, err := s.app.Autostart.IsEnabled(); err != nil {
		ui.AutostartErr = err.Error()
	} else {
		ui.Autostart = on
	}
	now, maxAge := time.Now(), core.MaxAge(set)
	rows := tray.MenuModel(st, set, now, maxAge, ui)
	percent, level := tray.IconSpec(st, maxAge, now)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.drawnAt = st.UpdatedAt
	application.InvokeSync(func() {
		s.menu.Clear()
		for _, r := range rows {
			s.add(r)
		}
		s.menu.Update()
	})
	s.tray.SetIcon(tray.IconPNG(percent, level))
	s.tray.SetTooltip(tray.TooltipText(st, maxAge, now))
}

func joinErr(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func (s *shell) add(r tray.Row) {
	var item *application.MenuItem
	switch r.Kind {
	case tray.Separator:
		s.menu.AddSeparator()
		return
	case tray.Label:
		s.menu.Add(r.Text).SetEnabled(false)
		return
	case tray.Check:
		item = s.menu.AddCheckbox(r.Text, r.Checked)
	default:
		item = s.menu.Add(r.Text)
	}
	item.SetEnabled(r.Enabled).OnClick(func(*application.Context) { go s.click(r) })
}

// click runs a row's action. Every outcome is drawn: core errors through
// State.LastError, the tray's own through a notification.
func (s *shell) click(r tray.Row) {
	sw := s.o.Switcher
	switch r.Action {
	case tray.ActUse:
		s.op("use "+r.Arg, opTimeout, func(ctx context.Context) (domain.State, error) { return sw.Switch(ctx, r.Arg) })
	case tray.ActAuto:
		s.op("auto-switch", opTimeout, func(ctx context.Context) (domain.State, error) { return sw.SetAuto(ctx, !r.Checked) })
	case tray.ActUndo:
		s.op("undo", opTimeout, sw.Undo)
	case tray.ActRefresh:
		s.op("refresh", opTimeout, func(ctx context.Context) (domain.State, error) { return sw.Poll(ctx, true) })
	case tray.ActUpdate:
		s.update()
	case tray.ActAdd:
		name, args := tray.AddCommand(runtime.GOOS, s.o.CLIPath)
		s.failed("Could not open a terminal for ccswitch add", s.o.Start(name, args...))
	case tray.ActOpenLog:
		s.failed("Could not open the log", s.app.Browser.OpenFile(s.o.LogPath))
	case tray.ActAutostart:
		s.autostart(!r.Checked)
	case tray.ActQuit:
		s.app.Quit()
	}
}

// update runs `claude update` with its row disabled, since a second run
// would race the first.
func (s *shell) update() {
	if !s.updating.CompareAndSwap(false, true) {
		return
	}
	s.redraw()
	defer func() {
		s.updating.Store(false)
		s.redraw()
	}()
	s.op("update", updateTimeout, s.o.Switcher.Update)
}

func (s *shell) autostart(on bool) {
	var err error
	if on {
		err = s.app.Autostart.Enable()
	} else {
		err = s.app.Autostart.Disable()
	}
	s.failed("Could not change start at login", err)
	s.redraw()
}

// failed logs and shows an error of the tray's own (not the core's).
func (s *shell) failed(title string, err error) {
	if err == nil {
		return
	}
	s.o.Logf("tray: %s: %v", title, err)
	if nerr := s.o.Notifier.Notify(title, err.Error()); nerr != nil {
		s.o.Logf("tray: %v", errors.Join(err, nerr))
	}
}
