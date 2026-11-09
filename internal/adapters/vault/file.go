// Package vault is the store of the saved accounts (ports.Vault) and the
// choice of it for this OS (Open): FileVault, accounts.json in the app data
// folder, sealed with DPAPI on Windows (NewDPAPI) and plain 0600 JSON on
// Linux; KeychainVault, one item in the login keychain, on macOS.
package vault

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

var _ ports.Vault = (*FileVault)(nil)

// FileVault keeps every account in one file, readable by the user only.
type FileVault struct {
	path         string
	seal, unseal func([]byte) ([]byte, error)
}

func same(b []byte) ([]byte, error) { return b, nil }

// NewFile returns the plain JSON vault at <dir>/accounts.json.
func NewFile(dir string) *FileVault {
	return NewSealedFile(dir, same, same)
}

// NewSealedFile returns the vault at <dir>/accounts.json whose bytes pass through
// seal on the way to disk and unseal on the way back (DPAPI on Windows).
func NewSealedFile(dir string, seal, unseal func([]byte) ([]byte, error)) *FileVault {
	return &FileVault{path: filepath.Join(dir, Name), seal: seal, unseal: unseal}
}

// Load returns the stored accounts; none when the file does not exist yet.
func (v *FileVault) Load() ([]domain.Account, error) {
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
func (v *FileVault) Save(accounts []domain.Account) error {
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
