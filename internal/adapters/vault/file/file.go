// Package file is the vault as accounts.json in the app data folder
// (ports.Vault): plain JSON, or sealed bytes when built with NewSealed.
package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/diegofal/ccswitch/internal/adapters/atomicfile"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Name is the vault's file name inside the data folder.
const Name = "accounts.json"

var _ ports.Vault = (*Vault)(nil)

// Vault keeps every account in one file, readable by the user only.
type Vault struct {
	path         string
	seal, unseal func([]byte) ([]byte, error)
}

func same(b []byte) ([]byte, error) { return b, nil }

// New returns the plain JSON vault at <dir>/accounts.json.
func New(dir string) *Vault {
	return NewSealed(dir, same, same)
}

// NewSealed returns the vault at <dir>/accounts.json whose bytes pass through
// seal on the way to disk and unseal on the way back (DPAPI on Windows).
func NewSealed(dir string, seal, unseal func([]byte) ([]byte, error)) *Vault {
	return &Vault{path: filepath.Join(dir, Name), seal: seal, unseal: unseal}
}

// Load returns the stored accounts; none when the file does not exist yet.
func (v *Vault) Load() ([]domain.Account, error) {
	b, err := os.ReadFile(v.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []domain.Account{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	if b, err = v.unseal(b); err != nil {
		return nil, fmt.Errorf("vault %s: %w", v.path, err)
	}
	var accounts []domain.Account
	if err := json.Unmarshal(b, &accounts); err != nil {
		return nil, fmt.Errorf("vault %s: %w", v.path, err)
	}
	if accounts == nil {
		accounts = []domain.Account{}
	}
	return accounts, nil
}

// Save replaces the stored accounts, creating the folder on first use.
func (v *Vault) Save(accounts []domain.Account) error {
	if accounts == nil {
		accounts = []domain.Account{}
	}
	b, err := json.Marshal(accounts)
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	if b, err = v.seal(b); err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	return atomicfile.Write(v.path, b, 0o600)
}
