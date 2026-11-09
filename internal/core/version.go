package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegofal/ccswitch/internal/domain"
)

// CheckVersion reads the installed and the latest Claude Code versions
// (PLAN D10). When the latest cannot be read the installed one is still
// recorded, and VersionReadAt is left alone so the next poll tries again.
func (sw *Switcher) CheckVersion(ctx context.Context) (domain.State, error) {
	return sw.run(ctx, sw.checkVersion)
}

func (sw *Switcher) checkVersion(s *session) error {
	installed, err := sw.installed(s)
	if err != nil {
		return err
	}
	s.st.Installed = installed
	latest, err := sw.d.Releases.Latest(s.ctx)
	if err != nil {
		return fmt.Errorf("latest Claude Code version: %w", err)
	}
	if latest, err = version(latest); err != nil {
		return fmt.Errorf("latest Claude Code version: %w", err)
	}
	s.st.Latest, s.st.VersionReadAt = latest, s.now
	return nil
}

func (sw *Switcher) installed(s *session) (string, error) {
	out, err := sw.d.Claude.Version(s.ctx)
	if err != nil {
		return "", fmt.Errorf("claude --version: %w", err)
	}
	v, err := version(out)
	if err != nil {
		return "", fmt.Errorf("claude --version: %w", err)
	}
	return v, nil
}

// version is "2.1.300" out of "2.1.300 (Claude Code)".
func version(s string) (string, error) {
	v, err := domain.ParseVersion(s)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]), nil
}

// Update runs `claude update` (PLAN D10), logs its output, reads the
// version again and says the result in a notification. Running sessions
// keep the old binary; new ones get the new.
func (sw *Switcher) Update(ctx context.Context) (domain.State, error) {
	return sw.run(ctx, func(s *session) error {
		out, err := sw.d.Claude.Update(s.ctx)
		sw.d.Log.Printf("claude update output:\n%s", strings.TrimSpace(out))
		if err == nil {
			var v string
			if v, err = sw.installed(s); err == nil {
				s.st.Installed = v
				sw.notify(s, "Claude Code updated", fmt.Sprintf("Claude Code %s installed — new sessions use it", v))
				return nil
			}
		} else {
			err = fmt.Errorf("claude update: %w", err)
		}
		sw.notify(s, "Claude Code update failed", err.Error())
		return err
	})
}
