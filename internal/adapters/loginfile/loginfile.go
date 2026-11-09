// Package loginfile is Claude Code's live login as files:
// ~/.claude/.credentials.json and ~/.claude.json (ports.LoginStore). Its
// ConfigFile is the .claude.json half on its own, which the macOS store
// (loginkeychain) shares.
package loginfile

import (
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/atomicfile"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

var _ ports.LoginStore = (*Store)(nil)

// Store reads and patches the two files at the given paths.
type Store struct {
	credentials jsonFile
	config      ConfigFile
}

// New returns the store over the credentials file (.credentials.json) and the
// config file (.claude.json).
func New(credentialsPath, configPath string) *Store {
	return &Store{credentials: jsonFile(credentialsPath), config: Config(configPath)}
}

// Read returns the login held in the two files.
func (s *Store) Read() (domain.Login, error) {
	creds, _, err := s.credentials.read()
	if err != nil {
		return domain.Login{}, err
	}
	config, err := s.config.Read()
	if err != nil {
		return domain.Login{}, err
	}
	return domain.LoginFromFiles(creds, config)
}

// Write patches l into both files. Both patches are made before either file
// is replaced, so a login the patch rejects leaves both files as they were.
func (s *Store) Write(l domain.Login) error {
	writeCreds, err := s.credentials.patch(l, domain.PatchCredentialsFile)
	if err != nil {
		return err
	}
	writeConfig, err := s.config.Patch(l)
	if err != nil {
		return err
	}
	if err := writeCreds(); err != nil {
		return err
	}
	return writeConfig()
}

// ModTime is the credentials file's modification time.
func (s *Store) ModTime() (time.Time, error) {
	fi, err := os.Stat(string(s.credentials))
	if err != nil {
		return time.Time{}, fmt.Errorf("claude login file: %w", err)
	}
	return fi.ModTime(), nil
}

// ConfigFile is Claude Code's config file (.claude.json), the half of the
// login that is a file on every OS.
type ConfigFile struct{ file jsonFile }

// Config returns the config file at path.
func Config(path string) ConfigFile { return ConfigFile{file: jsonFile(path)} }

// Read returns the file's bytes.
func (c ConfigFile) Read() ([]byte, error) {
	b, _, err := c.file.read()
	return b, err
}

// Patch reads the file and sets its oauthAccount to l's profile. It writes
// nothing: the returned write replaces the file atomically, mode kept, so a
// caller can prepare every part of a login before replacing any of them.
func (c ConfigFile) Patch(l domain.Login) (write func() error, err error) {
	return c.file.patch(l, domain.PatchConfigFile)
}

// jsonFile is one of Claude Code's JSON files.
type jsonFile string

func (f jsonFile) read() ([]byte, fs.FileMode, error) {
	b, err := os.ReadFile(string(f))
	if err != nil {
		return nil, 0, fmt.Errorf("claude login file: %w", err)
	}
	fi, err := os.Stat(string(f))
	if err != nil {
		return nil, 0, fmt.Errorf("claude login file: %w", err)
	}
	return b, fi.Mode().Perm(), nil
}

// patch reads the file and applies fn; the returned write puts the result
// in place with the file's mode.
func (f jsonFile) patch(l domain.Login, fn func([]byte, domain.Login) ([]byte, error)) (func() error, error) {
	b, mode, err := f.read()
	if err != nil {
		return nil, err
	}
	patched, err := fn(b, l)
	if err != nil {
		return nil, err
	}
	return func() error { return atomicfile.Write(string(f), patched, mode) }, nil
}
