// Package keychain is the vault on macOS (ports.Vault): every account as
// one JSON value in one generic password, "ccswitch-accounts", in the login
// keychain, through the `security` program (keychainitem). The keychain
// encrypts the whole value, so nothing is sealed here. It builds on every
// OS; `security` is injected. Untested on a real Mac (PLAN V5).
package keychain

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diegofal/ccswitch/internal/adapters/keychainitem"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Service is the keychain item's service name.
const Service = "ccswitch-accounts"

var _ ports.Vault = (*Vault)(nil)

// Vault is the one keychain item.
type Vault struct{ item keychainitem.Item }

// New returns the vault for the data folder dataDir, calling `security`
// through run. The folder is the item's account, so a test copy with its own
// CCSWITCH_DATA_DIR has its own item and never reaches the real vault.
func New(dataDir string, run keychainitem.Runner) *Vault {
	return &Vault{item: keychainitem.Item{Service: Service, Account: dataDir, Run: run}}
}

// Load returns the stored accounts; none when the item does not exist yet.
func (v *Vault) Load() ([]domain.Account, error) {
	b, err := v.item.Get()
	if errors.Is(err, keychainitem.ErrNotFound) {
		return []domain.Account{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	var accounts []domain.Account
	if err := json.Unmarshal(b, &accounts); err != nil {
		return nil, fmt.Errorf("vault: keychain item %q: %w", Service, err)
	}
	if accounts == nil {
		accounts = []domain.Account{}
	}
	return accounts, nil
}

// Save replaces the stored accounts, creating the item on first use. The
// JSON holds every account's tokens and `security` takes it as an argument
// (keychainitem.Item.Set).
func (v *Vault) Save(accounts []domain.Account) error {
	if accounts == nil {
		accounts = []domain.Account{}
	}
	b, err := json.Marshal(accounts)
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	if err := v.item.Set(b); err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	return nil
}
