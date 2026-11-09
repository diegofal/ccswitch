// Package loginfile is Claude Code's live login as files:
// ~/.claude/.credentials.json and ~/.claude.json (ports.LoginStore).
package loginfile

import (
	"fmt"
	"os"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/atomicfile"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

var _ ports.LoginStore = (*Store)(nil)

// Store reads and patches the two files at the given paths.
type Store struct {
	credentials, config string
}

// New returns the store over the credentials file (.credentials.json) and the
// config file (.claude.json).
func New(credentialsPath, configPath string) *Store {
	return &Store{credentials: credentialsPath, config: configPath}
}

func readFile(path string) ([]byte, os.FileMode, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("claude login file: %w", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("claude login file: %w", err)
	}
	return b, fi.Mode().Perm(), nil
}

// Read returns the login held in the two files.
func (s *Store) Read() (domain.Login, error) {
	creds, _, err := readFile(s.credentials)
	if err != nil {
		return domain.Login{}, err
	}
	config, _, err := readFile(s.config)
	if err != nil {
		return domain.Login{}, err
	}
	return domain.LoginFromFiles(creds, config)
}

// Write patches l into both files. Both patches are made before either file
// is replaced, so a login the patch rejects leaves both files as they were.
func (s *Store) Write(l domain.Login) error {
	creds, credsMode, err := readFile(s.credentials)
	if err != nil {
		return err
	}
	config, configMode, err := readFile(s.config)
	if err != nil {
		return err
	}
	newCreds, err := domain.PatchCredentialsFile(creds, l)
	if err != nil {
		return err
	}
	newConfig, err := domain.PatchConfigFile(config, l)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(s.credentials, newCreds, credsMode); err != nil {
		return err
	}
	return atomicfile.Write(s.config, newConfig, configMode)
}

// ModTime is the credentials file's modification time.
func (s *Store) ModTime() (time.Time, error) {
	fi, err := os.Stat(s.credentials)
	if err != nil {
		return time.Time{}, fmt.Errorf("claude login file: %w", err)
	}
	return fi.ModTime(), nil
}
