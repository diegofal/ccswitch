// Package usageapi is the claude.ai usage endpoint and the OAuth token
// refresh over HTTPS (ports.UsageAPI, PLAN F3, F4).
package usageapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Defaults are the real endpoints; tests point the client at httptest.
const (
	DefaultAPIBase     = "https://api.anthropic.com"
	DefaultConsoleBase = "https://console.anthropic.com"
	DefaultTimeout     = 20 * time.Second

	// clientID is Claude Code's public OAuth client: refresh tokens are
	// bound to it.
	clientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	// maxBody bounds what is read from any response.
	maxBody = 1 << 20
	// bodyInError is how much of an unexpected response an error carries.
	bodyInError = 200
)

var _ ports.UsageAPI = (*Client)(nil)

// Client calls the usage and token endpoints.
type Client struct {
	api, console string
	userAgent    string
	clock        ports.Clock
	http         *http.Client
}

// Option changes a Client built by New.
type Option func(*Client)

// WithAPIBase sets the base URL of the usage endpoint (default
// DefaultAPIBase).
func WithAPIBase(url string) Option {
	return func(c *Client) { c.api = strings.TrimRight(url, "/") }
}

// WithConsoleBase sets the base URL of the token endpoint (default
// DefaultConsoleBase).
func WithConsoleBase(url string) Option {
	return func(c *Client) { c.console = strings.TrimRight(url, "/") }
}

// WithTimeout sets the per-request timeout (default DefaultTimeout).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// New returns a client that sends User-Agent ccswitch/<version> and stamps
// reads and token expiries with clock.
func New(version string, clock ports.Clock, opts ...Option) *Client {
	c := &Client{
		api:       DefaultAPIBase,
		console:   DefaultConsoleBase,
		userAgent: "ccswitch/" + version,
		clock:     clock,
		http:      &http.Client{Timeout: DefaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Fetch reads the account's limits with its access token.
func (c *Client) Fetch(ctx context.Context, accessToken string) (domain.Usage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api+"/api/oauth/usage", http.NoBody)
	if err != nil {
		return domain.Usage{}, fmt.Errorf("usage: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	body, err := c.do(req, "usage", accessToken)
	if err != nil {
		return domain.Usage{}, err
	}
	return domain.ParseUsage(body, c.clock.Now())
}

// tokenResponse is the token endpoint's answer; RefreshToken is optional
// (the server may keep the old one).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Refresh trades a refresh token for new tokens.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (domain.Tokens, error) {
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     clientID,
	})
	if err != nil {
		return domain.Tokens{}, fmt.Errorf("token refresh: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.console+"/v1/oauth/token", bytes.NewReader(payload))
	if err != nil {
		return domain.Tokens{}, fmt.Errorf("token refresh: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := c.do(req, "token refresh", refreshToken)
	if err != nil {
		return domain.Tokens{}, err
	}
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return domain.Tokens{}, fmt.Errorf("token refresh: response is not JSON: %w", err)
	}
	if r.AccessToken == "" || r.ExpiresIn <= 0 {
		return domain.Tokens{}, errors.New("token refresh: response has no access_token or expires_in")
	}
	t := domain.Tokens{
		Access:    r.AccessToken,
		Refresh:   r.RefreshToken,
		ExpiresAt: c.clock.Now().Add(time.Duration(r.ExpiresIn) * time.Second),
	}
	if t.Refresh == "" {
		t.Refresh = refreshToken
	}
	return t, nil
}

// do sends req and returns the body of a 200. secret is the token the
// request carries: it is cut out of anything an error repeats.
func (c *Client) do(req *http.Request, what, secret string) ([]byte, error) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", what, redact(err.Error(), secret))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	_ = resp.Body.Close() // the body is read; a close error changes nothing
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %s", what, redact(err.Error(), secret))
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%s: HTTP %d: %w", what, resp.StatusCode, ports.ErrUnauthorized)
	default:
		return nil, fmt.Errorf("%s: HTTP %d: %s", what, resp.StatusCode, excerpt(body, secret))
	}
}

// excerpt is the start of a response body, on one line, without the secret.
func excerpt(body []byte, secret string) string {
	s := strings.Join(strings.Fields(redact(string(body), secret)), " ")
	if r := []rune(s); len(r) > bodyInError {
		s = string(r[:bodyInError]) + "…"
	}
	return s
}

func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[token]")
}
