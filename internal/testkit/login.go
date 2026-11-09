package testkit

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

// T0 is the fixed "now" of the fixtures: Monday 2026-11-09 12:00 UTC.
var T0 = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// FakeLogin is a login shaped like Claude Code's, with made-up tokens
// "fake-access-<email>" and "fake-refresh-<email>", an access token expiring
// 12 h after T0 and a refresh token 28 days after.
func FakeLogin(email string) domain.Login {
	q := func(s string) string {
		b, _ := json.Marshal(s) // a Go string always marshals
		return string(b)
	}
	ms := func(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }
	org := q("fake-org-" + email)
	return domain.Login{
		Credentials: json.RawMessage(`{"accessToken":` + q("fake-access-"+email) +
			`,"refreshToken":` + q("fake-refresh-"+email) +
			`,"expiresAt":` + ms(T0.Add(12*time.Hour)) +
			`,"refreshTokenExpiresAt":` + ms(T0.Add(28*24*time.Hour)) +
			`,"scopes":["user:inference","user:profile"],"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}`),
		OrganizationUUID: json.RawMessage(org),
		Profile: json.RawMessage(`{"accountUuid":` + q("fake-account-"+email) +
			`,"emailAddress":` + q(email) +
			`,"organizationUuid":` + org +
			`,"organizationName":"Fake Org","planDisplayName":"Max 20x"}`),
	}
}
