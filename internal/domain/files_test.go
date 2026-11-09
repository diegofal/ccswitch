package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

// rawMember returns the raw value of a top-level member as it sits in file.
func rawMember(t *testing.T, file []byte, key string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(file, &m); err != nil {
		t.Fatal(err)
	}
	v, ok := m[key]
	if !ok {
		t.Fatalf("no member %q", key)
	}
	return string(v)
}

func TestPatchCredentialsFileKeepsEveryOtherByte(t *testing.T) {
	file := testkit.Fixture(t, "credentials.json")
	got, err := domain.PatchCredentialsFile(file, login("new@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	newCreds := `{
    "accessToken": "fake-access-new@example.com",
    "refreshToken": "fake-refresh-new@example.com",
    "expiresAt": 1794268800000,
    "refreshTokenExpiresAt": 1796601600000,
    "scopes": [
      "user:inference"
    ],
    "subscriptionType": "pro"
  }`
	want := strings.NewReplacer(
		rawMember(t, file, "claudeAiOauth"), newCreds,
		`"00000000-0000-4000-8000-00000000000a"`, `"00000000-0000-4000-8000-00000000000b"`,
	).Replace(string(file))
	if string(got) != want {
		t.Errorf("patched file differs outside claudeAiOauth and organizationUuid:\n got %s\nwant %s", got, want)
	}
	if !json.Valid(got) {
		t.Error("output is not valid JSON")
	}
}

func TestPatchFilesRoundTripIsIdentity(t *testing.T) {
	creds, config := testkit.Fixture(t, "credentials.json"), testkit.Fixture(t, "claude.json")
	l := liveLogin(t)
	if got, err := domain.PatchCredentialsFile(creds, l); err != nil || string(got) != string(creds) {
		t.Errorf("re-writing the live login changed .credentials.json (%v):\n%s", err, got)
	}
	if got, err := domain.PatchConfigFile(config, l); err != nil || string(got) != string(config) {
		t.Errorf("re-writing the live login changed .claude.json (%v):\n%s", err, got)
	}
}

func TestPatchConfigFileKeepsEveryOtherByte(t *testing.T) {
	file := testkit.Fixture(t, "claude.json")
	got, err := domain.PatchConfigFile(file, login("new@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	newProfile := `{
    "accountUuid": "fake-uuid",
    "emailAddress": "new@example.com",
    "organizationName": "New Org",
    "planDisplayName": "Pro"
  }`
	want := strings.Replace(string(file), rawMember(t, file, "oauthAccount"), newProfile, 1)
	if string(got) != want {
		t.Errorf("patched file differs outside oauthAccount:\n got %s\nwant %s", got, want)
	}
}

func TestPatchFileShapes(t *testing.T) {
	l := domain.Login{
		Credentials:      json.RawMessage(`{"accessToken":"a"}`),
		OrganizationUUID: json.RawMessage(`"org"`),
		Profile:          json.RawMessage(`{"emailAddress":"a@b.c"}`),
	}
	noOrg := l
	noOrg.OrganizationUUID = nil
	cases := []struct {
		name  string
		patch func([]byte, domain.Login) ([]byte, error)
		l     domain.Login
		in    string
		want  string
	}{
		{"absent members appended in order", domain.PatchCredentialsFile, l,
			"{\n  \"mcpOAuth\": {}\n}\n",
			"{\n  \"mcpOAuth\": {},\n  \"claudeAiOauth\": {\n    \"accessToken\": \"a\"\n  },\n  \"organizationUuid\": \"org\"\n}\n"},
		{"empty object", domain.PatchConfigFile, l,
			"{}",
			"{\n  \"oauthAccount\": {\n    \"emailAddress\": \"a@b.c\"\n  }\n}"},
		{"compact file stays compact", domain.PatchCredentialsFile, l,
			`{"claudeAiOauth":{"accessToken":"old"},"x":1}`,
			`{"claudeAiOauth":{"accessToken":"a"},"x":1,"organizationUuid":"org"}`},
		{"nil organization leaves the member alone", domain.PatchCredentialsFile, noOrg,
			"{\n  \"organizationUuid\": \"keep\",\n  \"claudeAiOauth\": null\n}",
			"{\n  \"organizationUuid\": \"keep\",\n  \"claudeAiOauth\": {\n    \"accessToken\": \"a\"\n  }\n}"},
		{"duplicate keys all replaced", domain.PatchConfigFile, l,
			`{"oauthAccount":1,"oauthAccount":2}`,
			`{"oauthAccount":{"emailAddress":"a@b.c"},"oauthAccount":{"emailAddress":"a@b.c"}}`},
		{"escaped key and leading space", domain.PatchConfigFile, l,
			"  {\"a\\\"b\" : 1}",
			"  {\"a\\\"b\" : 1,\"oauthAccount\" : {\"emailAddress\":\"a@b.c\"}}"},
	}
	for _, c := range cases {
		got, err := c.patch([]byte(c.in), c.l)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		if !json.Valid(got) {
			t.Errorf("%s: output is not valid JSON", c.name)
		}
	}
}

func TestPatchFileErrors(t *testing.T) {
	good := login("a@b.c")
	cases := []struct {
		name  string
		patch func([]byte, domain.Login) ([]byte, error)
		file  string
		l     domain.Login
	}{
		{"array at top level", domain.PatchCredentialsFile, `[]`, good},
		{"string at top level", domain.PatchConfigFile, `"x"`, good},
		{"invalid JSON", domain.PatchCredentialsFile, `{"a":`, good},
		{"trailing garbage", domain.PatchConfigFile, `{} {}`, good},
		{"empty file", domain.PatchConfigFile, ``, good},
		{"no credentials", domain.PatchCredentialsFile, `{}`, domain.Login{Profile: good.Profile}},
		{"credentials not an object", domain.PatchCredentialsFile, `{}`, domain.Login{Credentials: json.RawMessage(`"x"`)}},
		{"organization not JSON", domain.PatchCredentialsFile, `{}`, domain.Login{Credentials: good.Credentials, OrganizationUUID: json.RawMessage(`nope`)}},
		{"no profile", domain.PatchConfigFile, `{}`, domain.Login{Credentials: good.Credentials}},
		{"profile not JSON", domain.PatchConfigFile, `{}`, domain.Login{Profile: json.RawMessage(`{`)}},
	}
	for _, c := range cases {
		if got, err := c.patch([]byte(c.file), c.l); err == nil {
			t.Errorf("%s: want error, got %s", c.name, got)
		}
	}
}
