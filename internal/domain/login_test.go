package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

var t0 = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// login builds a Login the way LoginFromFiles returns one, with fake tokens.
func login(email string) domain.Login {
	return domain.Login{
		Credentials: json.RawMessage(`{"accessToken":"fake-access-` + email + `","refreshToken":"fake-refresh-` + email +
			`","expiresAt":1794268800000,"refreshTokenExpiresAt":1796601600000,"scopes":["user:inference"],"subscriptionType":"pro"}`),
		OrganizationUUID: json.RawMessage(`"00000000-0000-4000-8000-00000000000b"`),
		Profile:          json.RawMessage(`{"accountUuid":"fake-uuid","emailAddress":"` + email + `","organizationName":"New Org","planDisplayName":"Pro"}`),
	}
}

func liveLogin(t *testing.T) domain.Login {
	t.Helper()
	l, err := domain.LoginFromFiles(testkit.Fixture(t, "credentials.json"), testkit.Fixture(t, "claude.json"))
	if err != nil {
		t.Fatalf("LoginFromFiles: %v", err)
	}
	return l
}

func TestLoginFromFiles(t *testing.T) {
	l := liveLogin(t)
	if !strings.HasPrefix(string(l.Credentials), `{
    "accessToken": "fake-access-token-old",`) || !strings.HasSuffix(string(l.Credentials), `"rateLimitTier": "default_claude_max_20x"
  }`) {
		t.Errorf("Credentials is not the raw claudeAiOauth value:\n%s", l.Credentials)
	}
	if got := string(l.OrganizationUUID); got != `"00000000-0000-4000-8000-00000000000a"` {
		t.Errorf("OrganizationUUID = %s", got)
	}
	if !strings.Contains(string(l.Profile), `"emailAddress": "Old.User@Example.com"`) {
		t.Errorf("Profile is not the raw oauthAccount value:\n%s", l.Profile)
	}

	noOrg, err := domain.LoginFromFiles([]byte(`{"claudeAiOauth":{"accessToken":"a"}}`), []byte(`{"oauthAccount":{"emailAddress":"a@b.c"}}`))
	if err != nil || noOrg.OrganizationUUID != nil {
		t.Errorf("absent organizationUuid: got %s, %v; want nil, nil", noOrg.OrganizationUUID, err)
	}

	bad := []struct{ name, creds, config string }{
		{"credentials not JSON", `{`, `{"oauthAccount":{}}`},
		{"config not JSON", `{"claudeAiOauth":{}}`, `nope`},
		{"credentials not an object", `[]`, `{"oauthAccount":{}}`},
		{"no claudeAiOauth", `{"mcpOAuth":{}}`, `{"oauthAccount":{}}`},
		{"claudeAiOauth null", `{"claudeAiOauth":null}`, `{"oauthAccount":{}}`},
		{"no oauthAccount", `{"claudeAiOauth":{}}`, `{"userID":"x"}`},
		{"oauthAccount not an object", `{"claudeAiOauth":{}}`, `{"oauthAccount":"x"}`},
	}
	for _, c := range bad {
		if _, err := domain.LoginFromFiles([]byte(c.creds), []byte(c.config)); err == nil {
			t.Errorf("%s: want error", c.name)
		}
	}
}

func TestLoginEmail(t *testing.T) {
	cases := []struct {
		name, profile, want string
		err                 bool
	}{
		{"lower-cased and trimmed", `{"emailAddress":"  Old.User@Example.COM "}`, "old.user@example.com", false},
		{"missing", `{"organizationName":"x"}`, "", true},
		{"blank", `{"emailAddress":"  "}`, "", true},
		{"not a string", `{"emailAddress":3}`, "", true},
		{"not JSON", `{`, "", true},
	}
	for _, c := range cases {
		got, err := domain.Login{Profile: json.RawMessage(c.profile)}.Email()
		if got != c.want || (err != nil) != c.err {
			t.Errorf("%s: Email() = %q, %v; want %q, error %v", c.name, got, err, c.want, c.err)
		}
	}
}

func TestAccountFromLogin(t *testing.T) {
	a, err := domain.AccountFromLogin(liveLogin(t), t0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Email != "old.user@example.com" || a.OrgName != "Old Org" || a.Plan != "Max 20x" || !a.InPool || !a.CapturedAt.Equal(t0) {
		t.Errorf("AccountFromLogin = %+v", a)
	}

	l := login("b@example.com")
	l.Profile = json.RawMessage(`{"emailAddress":"b@example.com"}`)
	b, err := domain.AccountFromLogin(l, t0)
	if err != nil || b.Plan != "pro" || b.OrgName != "" {
		t.Errorf("plan falls back to subscriptionType: got %+v, %v", b, err)
	}

	bad := []domain.Login{
		{Credentials: login("a@b.c").Credentials, Profile: json.RawMessage(`{}`)},
		{Credentials: json.RawMessage(`[]`), Profile: login("a@b.c").Profile},
	}
	for i, l := range bad {
		if _, err := domain.AccountFromLogin(l, t0); err == nil {
			t.Errorf("bad login %d: want error", i)
		}
	}
}

func account(creds string) domain.Account {
	return domain.Account{Email: "a@b.c", Login: domain.Login{Credentials: json.RawMessage(creds)}}
}

func TestAccountTokens(t *testing.T) {
	a, err := domain.AccountFromLogin(liveLogin(t), t0)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := a.AccessToken(); v != "fake-access-token-old" || err != nil {
		t.Errorf("AccessToken = %q, %v", v, err)
	}
	if v, err := a.RefreshToken(); v != "fake-refresh-token-old" || err != nil {
		t.Errorf("RefreshToken = %q, %v", v, err)
	}
	if v, err := a.ExpiresAt(); !v.Equal(time.UnixMilli(1794225600000)) || err != nil {
		t.Errorf("ExpiresAt = %v, %v", v, err)
	}
	if v, err := a.RefreshExpiresAt(); !v.Equal(time.UnixMilli(1796601600000)) || err != nil {
		t.Errorf("RefreshExpiresAt = %v, %v", v, err)
	}

	if v, err := account(`{"accessToken":"x"}`).RefreshExpiresAt(); !v.IsZero() || err != nil {
		t.Errorf("absent refreshTokenExpiresAt = %v, %v; want zero, nil", v, err)
	}
	if v, err := account(`{"expiresAt":1.7942256e12}`).ExpiresAt(); !v.Equal(time.UnixMilli(1794225600000)) || err != nil {
		t.Errorf("float expiresAt = %v, %v", v, err)
	}

	broken := []string{`{`, `{"accessToken":""}`, `{"accessToken":3}`, `{"refreshToken":null}`, `{"expiresAt":"soon"}`, `{"refreshTokenExpiresAt":"x"}`}
	for _, c := range broken {
		a := account(c)
		_, e1 := a.AccessToken()
		_, e2 := a.RefreshToken()
		_, e3 := a.ExpiresAt()
		if e1 == nil || e2 == nil || e3 == nil {
			t.Errorf("%s: want errors from AccessToken, RefreshToken, ExpiresAt; got %v, %v, %v", c, e1, e2, e3)
		}
	}
	if _, err := account(`{"refreshTokenExpiresAt":"x"}`).RefreshExpiresAt(); err == nil {
		t.Error("invalid refreshTokenExpiresAt: want error")
	}
	if _, err := account(`{`).RefreshExpiresAt(); err == nil {
		t.Error("invalid credentials: want error from RefreshExpiresAt")
	}
}

func TestAccountExpired(t *testing.T) {
	exp := time.UnixMilli(1794225600000)
	a := account(`{"expiresAt":1794225600000}`)
	cases := []struct {
		name string
		a    domain.Account
		now  time.Time
		want bool
	}{
		{"before", a, exp.Add(-time.Second), false},
		{"at expiry", a, exp, true},
		{"after", a, exp.Add(time.Hour), true},
		{"unreadable expiresAt", account(`{}`), exp.Add(-time.Hour), true},
	}
	for _, c := range cases {
		if got := c.a.Expired(c.now); got != c.want {
			t.Errorf("%s: Expired = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAccountWithTokens(t *testing.T) {
	a, err := domain.AccountFromLogin(liveLogin(t), t0)
	if err != nil {
		t.Fatal(err)
	}
	exp := time.UnixMilli(1794300000000)
	b, err := a.WithTokens(domain.Tokens{Access: "fake-access-new", Refresh: "fake-refresh-new", ExpiresAt: exp})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(`"fake-access-token-old"`, `"fake-access-new"`, `"fake-refresh-token-old"`, `"fake-refresh-new"`,
		`"expiresAt": 1794225600000`, `"expiresAt": 1794300000000`).Replace(string(a.Login.Credentials))
	if string(b.Login.Credentials) != want {
		t.Errorf("WithTokens changed more than the three tokens:\n got %s\nwant %s", b.Login.Credentials, want)
	}
	if string(a.Login.Credentials) == want {
		t.Error("WithTokens modified the receiver")
	}
	if b.Email != a.Email || string(b.Login.Profile) != string(a.Login.Profile) {
		t.Error("WithTokens changed the account's other fields")
	}

	appended, err := account(`{"scopes":["x"]}`).WithTokens(domain.Tokens{Access: "a", Refresh: "r", ExpiresAt: exp})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(appended.Login.Credentials), `{"scopes":["x"],"accessToken":"a","refreshToken":"r","expiresAt":1794300000000}`; got != want {
		t.Errorf("absent members appended:\n got %s\nwant %s", got, want)
	}

	bad := []domain.Tokens{
		{Refresh: "r", ExpiresAt: exp},
		{Access: "a", ExpiresAt: exp},
		{Access: "a", Refresh: "r"},
	}
	for _, tok := range bad {
		if _, err := a.WithTokens(tok); err == nil {
			t.Errorf("WithTokens(%+v): want error", tok)
		}
	}
	if _, err := account(`[1]`).WithTokens(domain.Tokens{Access: "a", Refresh: "r", ExpiresAt: exp}); err == nil {
		t.Error("credentials not an object: want error")
	}
}
