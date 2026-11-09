//go:build windows

// The vault on Windows: accounts.json sealed with DPAPI for the current
// user, so another user or another machine cannot read the tokens (PLAN D4).

package vault

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/diegofal/ccswitch/internal/ports"
)

// entropy ties the blobs to ccswitch: another program running as the same
// user cannot unseal them with a bare DPAPI call.
var entropy = []byte("ccswitch")

// NewDPAPI returns the DPAPI-sealed vault at <dir>/accounts.json.
func NewDPAPI(dir string) (ports.Vault, error) {
	return NewSealedFile(dir, protect, unprotect), nil
}

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// take copies DPAPI's output into Go memory and frees DPAPI's.
func take(out *windows.DataBlob) []byte {
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

func protect(b []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(b), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("DPAPI seal: %w", err)
	}
	return take(&out), nil
}

func unprotect(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("DPAPI unseal: empty file")
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(b), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("DPAPI unseal: %w", err)
	}
	return take(&out), nil
}
