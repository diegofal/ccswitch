// Package npmreg asks the npm registry for the latest published Claude Code
// version (ports.Releases).
package npmreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/ports"
)

// DefaultBase is the public npm registry.
const DefaultBase = "https://registry.npmjs.org"

const (
	pkgPath = "/@anthropic-ai/claude-code/latest"
	timeout = 20 * time.Second
	maxBody = 1 << 20
)

var _ ports.Releases = (*Registry)(nil)

// Registry reads one package's latest dist-tag.
type Registry struct {
	base, userAgent string
	http            *http.Client
}

// New returns a registry client at base ("" = DefaultBase) that sends
// User-Agent ccswitch/<version>.
func New(base, version string) *Registry {
	if base == "" {
		base = DefaultBase
	}
	return &Registry{base: strings.TrimRight(base, "/"), userAgent: "ccswitch/" + version,
		http: &http.Client{Timeout: timeout}}
}

// Latest returns the version the registry's latest tag points at.
func (r *Registry) Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+pkgPath, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("npm registry: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", r.userAgent)
	resp, err := r.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("npm registry: %w", err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	_ = resp.Body.Close() // the body is read; a close error changes nothing
	if err != nil {
		return "", fmt.Errorf("npm registry: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("npm registry: response is not JSON: %w", err)
	}
	if doc.Version == "" {
		return "", errors.New("npm registry: response has no version")
	}
	return doc.Version, nil
}
