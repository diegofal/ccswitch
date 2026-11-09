// Package domain is ccswitch's pure logic: accounts and their logins, the
// patches to Claude Code's files, usage, the auto-switch decision, alerts and
// versions. No IO and no time.Now: callers pass the clock in.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Login is what Claude Code keeps for the active account: raw JSON values,
// opaque to us, so fields Claude Code adds later pass through untouched.
type Login struct {
	Credentials      json.RawMessage // the `claudeAiOauth` object of ~/.claude/.credentials.json
	OrganizationUUID json.RawMessage // the raw `organizationUuid` value of that file; nil if absent
	Profile          json.RawMessage // the `oauthAccount` object of ~/.claude.json
}

// profile is the part of oauthAccount ccswitch reads.
type profile struct {
	EmailAddress     string `json:"emailAddress"`
	OrganizationName string `json:"organizationName"`
	PlanDisplayName  string `json:"planDisplayName"`
	// SeatTier is null for an individual subscription; any other value is
	// read as a seat in a Team or Enterprise organisation (PLAN D14). Raw:
	// its shape for a seat has not been seen.
	SeatTier json.RawMessage `json:"seatTier"`
}

// seat reports whether the profile is a Team or Enterprise seat.
func (p profile) seat() bool {
	v := strings.TrimSpace(string(p.SeatTier))
	return v != "" && v != "null" && v != `""`
}

func (l Login) profile() (profile, error) {
	var p profile
	if err := json.Unmarshal(l.Profile, &p); err != nil {
		return profile{}, fmt.Errorf("oauthAccount: %w", err)
	}
	return p, nil
}

// Email is the profile's emailAddress, lower-cased and trimmed: the key
// accounts are stored under.
func (l Login) Email() (string, error) {
	p, err := l.profile()
	if err != nil {
		return "", err
	}
	email := strings.ToLower(strings.TrimSpace(p.EmailAddress))
	if email == "" {
		return "", errors.New("oauthAccount has no emailAddress")
	}
	return email, nil
}

// Account is one stored login and how ccswitch shows and uses it.
type Account struct {
	Email      string // key; lower-case
	OrgName    string // Profile.organizationName (display)
	Plan       string // Profile.planDisplayName, falling back to Credentials.subscriptionType (display)
	Login      Login
	InPool     bool // may be an auto-switch target
	CapturedAt time.Time
}

// credentials is the part of claudeAiOauth ccswitch reads.
type credentials struct {
	AccessToken           *string      `json:"accessToken"`
	RefreshToken          *string      `json:"refreshToken"`
	ExpiresAt             *json.Number `json:"expiresAt"`
	RefreshTokenExpiresAt *json.Number `json:"refreshTokenExpiresAt"`
	SubscriptionType      string       `json:"subscriptionType"`
}

func (l Login) credentials() (credentials, error) {
	var c credentials
	if !isObject(l.Credentials) {
		return c, errors.New("claudeAiOauth is not a JSON object")
	}
	if err := json.Unmarshal(l.Credentials, &c); err != nil {
		return c, fmt.Errorf("claudeAiOauth: %w", err)
	}
	return c, nil
}

// AccountFromLogin makes a new account from a login. It starts in the
// auto-switch pool unless it is a Team or Enterprise seat (PLAN D14).
func AccountFromLogin(l Login, now time.Time) (Account, error) {
	email, err := l.Email()
	if err != nil {
		return Account{}, err
	}
	p, _ := l.profile() // Email already parsed it
	c, err := l.credentials()
	if err != nil {
		return Account{}, err
	}
	plan := p.PlanDisplayName
	if plan == "" {
		plan = c.SubscriptionType
	}
	return Account{Email: email, OrgName: p.OrganizationName, Plan: plan, Login: l, InPool: !p.seat(), CapturedAt: now}, nil
}

func requiredString(v *string, name string) (string, error) {
	if v == nil || *v == "" {
		return "", fmt.Errorf("claudeAiOauth has no %s", name)
	}
	return *v, nil
}

// AccessToken is Credentials.accessToken.
func (a Account) AccessToken() (string, error) {
	c, err := a.Login.credentials()
	if err != nil {
		return "", err
	}
	return requiredString(c.AccessToken, "accessToken")
}

// RefreshToken is Credentials.refreshToken.
func (a Account) RefreshToken() (string, error) {
	c, err := a.Login.credentials()
	if err != nil {
		return "", err
	}
	return requiredString(c.RefreshToken, "refreshToken")
}

// millis reads a time stored as milliseconds since the epoch. JavaScript
// writes integers, but a float literal is still a number of milliseconds.
func millis(n json.Number, name string) (time.Time, error) {
	if ms, err := n.Int64(); err == nil {
		return time.UnixMilli(ms), nil
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("claudeAiOauth %s: %w", name, err)
	}
	return time.UnixMilli(int64(f)), nil
}

// ExpiresAt is when the access token expires (Credentials.expiresAt).
func (a Account) ExpiresAt() (time.Time, error) {
	c, err := a.Login.credentials()
	if err != nil {
		return time.Time{}, err
	}
	if c.ExpiresAt == nil {
		return time.Time{}, errors.New("claudeAiOauth has no expiresAt")
	}
	return millis(*c.ExpiresAt, "expiresAt")
}

// RefreshExpiresAt is when the refresh token dies; zero time and nil when
// the login does not say.
func (a Account) RefreshExpiresAt() (time.Time, error) {
	c, err := a.Login.credentials()
	if err != nil {
		return time.Time{}, err
	}
	if c.RefreshTokenExpiresAt == nil {
		return time.Time{}, nil
	}
	return millis(*c.RefreshTokenExpiresAt, "refreshTokenExpiresAt")
}

// Expired reports whether the access token is expired at now. A login whose
// expiry cannot be read counts as expired: it gets refreshed, not trusted.
func (a Account) Expired(now time.Time) bool {
	exp, err := a.ExpiresAt()
	return err != nil || !now.Before(exp)
}

// Tokens is the result of a token refresh.
type Tokens struct {
	Access, Refresh string
	ExpiresAt       time.Time
}

// WithTokens returns the account with accessToken, refreshToken and
// expiresAt replaced inside Credentials; every other member of the object
// keeps its raw bytes and order. All three are required: an empty token would
// lose the login.
func (a Account) WithTokens(t Tokens) (Account, error) {
	if t.Access == "" || t.Refresh == "" || t.ExpiresAt.IsZero() {
		return Account{}, errors.New("refreshed tokens are incomplete")
	}
	access, _ := json.Marshal(t.Access) // strings always marshal
	refresh, _ := json.Marshal(t.Refresh)
	creds, err := setMembers(a.Login.Credentials, []field{
		{"accessToken", access},
		{"refreshToken", refresh},
		{"expiresAt", []byte(strconv.FormatInt(t.ExpiresAt.UnixMilli(), 10))},
	})
	if err != nil {
		return Account{}, fmt.Errorf("claudeAiOauth: %w", err)
	}
	a.Login.Credentials = creds
	return a, nil
}
