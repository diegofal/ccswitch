package usageapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/usageapi"
	"github.com/diegofal/ccswitch/internal/ports"
	"github.com/diegofal/ccswitch/internal/testkit"
)

const (
	access  = "fake-access-token-secret"
	refresh = "fake-refresh-token-secret"
)

func client(t *testing.T, h http.HandlerFunc, opts ...usageapi.Option) *usageapi.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts = append([]usageapi.Option{usageapi.WithAPIBase(srv.URL + "/"), usageapi.WithConsoleBase(srv.URL)}, opts...)
	return usageapi.New("1.2.3", &testkit.FakeClock{T: testkit.T0}, opts...)
}

// noToken fails the test when err carries either token.
func noToken(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), access) || strings.Contains(err.Error(), refresh) {
		t.Fatalf("error leaks a token: %v", err)
	}
}

func TestFetch(t *testing.T) {
	body, err := os.ReadFile("testdata/usage-max20x.json")
	if err != nil {
		t.Fatal(err)
	}
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		want := map[string]string{
			"Authorization":  "Bearer " + access,
			"anthropic-beta": "oauth-2025-04-20",
			"Accept":         "application/json",
			"User-Agent":     "ccswitch/1.2.3",
		}
		for k, v := range want {
			if got := r.Header.Get(k); got != v {
				t.Errorf("header %s = %q, want %q", k, got, v)
			}
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/oauth/usage" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write(body)
	})
	u, err := c.Fetch(context.Background(), access)
	if err != nil {
		t.Fatal(err)
	}
	if !u.ReadAt.Equal(testkit.T0) || len(u.Limits) == 0 || u.Err != "" {
		t.Fatalf("usage %+v", u)
	}
}

func status(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

func TestFetchUnauthorized(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		_, err := client(t, status(code, `{"error":"bad token `+access+`"}`)).Fetch(context.Background(), access)
		noToken(t, err)
		if !errors.Is(err, ports.ErrUnauthorized) {
			t.Fatalf("HTTP %d: %v, want ErrUnauthorized", code, err)
		}
	}
}

func TestFetchServerError(t *testing.T) {
	body := "upstream said " + access + " " + strings.Repeat("x", 500)
	_, err := client(t, status(http.StatusInternalServerError, body)).Fetch(context.Background(), access)
	noToken(t, err)
	if errors.Is(err, ports.ErrUnauthorized) || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error %v, want HTTP 500", err)
	}
	if len(err.Error()) > 300 {
		t.Fatalf("error carries the whole body: %d chars", len(err.Error()))
	}
}

func TestFetchBadJSON(t *testing.T) {
	_, err := client(t, status(http.StatusOK, "<html>")).Fetch(context.Background(), access)
	noToken(t, err)
}

func TestFetchTimeout(t *testing.T) {
	release := make(chan struct{})
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}, usageapi.WithTimeout(50*time.Millisecond))
	defer close(release)
	_, err := c.Fetch(context.Background(), access)
	noToken(t, err)
}

func TestFetchUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c := usageapi.New("1.2.3", &testkit.FakeClock{T: testkit.T0}, usageapi.WithAPIBase(srv.URL))
	_, err := c.Fetch(context.Background(), access)
	noToken(t, err)
	_, err = c.Fetch(context.Background(), "bad\ntoken")
	if err == nil {
		t.Fatal("want an error for a token that is not a header value")
	}
}

func TestBadBaseURL(t *testing.T) {
	c := usageapi.New("1.2.3", &testkit.FakeClock{}, usageapi.WithAPIBase("::"), usageapi.WithConsoleBase("::"))
	if _, err := c.Fetch(context.Background(), access); err == nil {
		t.Fatal("Fetch: want an error for a bad base URL")
	}
	if _, err := c.Refresh(context.Background(), refresh); err == nil {
		t.Fatal("Refresh: want an error for a bad base URL")
	}
}

func TestRefresh(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/oauth/token" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("User-Agent") != "ccswitch/1.2.3" {
			t.Errorf("headers %v", r.Header)
		}
		var got map[string]string
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		want := map[string]string{"grant_type": "refresh_token", "refresh_token": refresh,
			"client_id": "9d1c250a-e61b-44d9-88ed-5944d1962f5e"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("body %s = %q, want %q", k, got[k], v)
			}
		}
		_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":28800,"token_type":"Bearer"}`)
	})
	tok, err := c.Refresh(context.Background(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != "new-access" || tok.Refresh != "new-refresh" || !tok.ExpiresAt.Equal(testkit.T0.Add(8*time.Hour)) {
		t.Fatalf("tokens %+v", tok)
	}
}

func TestRefreshKeepsOldRefreshToken(t *testing.T) {
	c := client(t, status(http.StatusOK, `{"access_token":"new-access","expires_in":60}`))
	tok, err := c.Refresh(context.Background(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Refresh != refresh {
		t.Fatalf("refresh token %q, want the old one kept", tok.Refresh)
	}
}

func TestRefreshFailures(t *testing.T) {
	cases := map[string]struct {
		h            http.HandlerFunc
		unauthorized bool
	}{
		"401":             {status(http.StatusUnauthorized, `{"error":"invalid_grant"}`), true},
		"403":             {status(http.StatusForbidden, ""), true},
		"500 echoing":     {status(http.StatusInternalServerError, `{"echo":"`+refresh+`"}`), false},
		"not JSON":        {status(http.StatusOK, "<html>"), false},
		"no access":       {status(http.StatusOK, `{"refresh_token":"x","expires_in":60}`), false},
		"no expires_in":   {status(http.StatusOK, `{"access_token":"x"}`), false},
		"wrong JSON type": {status(http.StatusOK, `{"access_token":"x","expires_in":"soon"}`), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client(t, tc.h).Refresh(context.Background(), refresh)
			noToken(t, err)
			if errors.Is(err, ports.ErrUnauthorized) != tc.unauthorized {
				t.Fatalf("error %v, unauthorized want %v", err, tc.unauthorized)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	if usageapi.DefaultAPIBase != "https://api.anthropic.com" || usageapi.DefaultConsoleBase != "https://console.anthropic.com" ||
		usageapi.DefaultTimeout != 20*time.Second {
		t.Fatal("defaults changed")
	}
}
