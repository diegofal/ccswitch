package npmreg_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/npmreg"
)

func registry(t *testing.T, code int, body string) *npmreg.Registry {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@anthropic-ai/claude-code/latest" || r.Header.Get("User-Agent") != "ccswitch/1.2.3" {
			t.Errorf("request %s, User-Agent %q", r.URL.Path, r.Header.Get("User-Agent"))
		}
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return npmreg.New(srv.URL+"/", "1.2.3")
}

func TestLatest(t *testing.T) {
	v, err := registry(t, http.StatusOK, `{"name":"@anthropic-ai/claude-code","version":"2.1.300","dist":{}}`).Latest(context.Background())
	if err != nil || v != "2.1.300" {
		t.Fatalf("Latest %q, %v", v, err)
	}
}

func TestLatestFailures(t *testing.T) {
	cases := map[string]struct {
		code int
		body string
		want string
	}{
		"5xx":        {http.StatusBadGateway, "oops", "HTTP 502"},
		"not found":  {http.StatusNotFound, `{"error":"not found"}`, "HTTP 404"},
		"bad JSON":   {http.StatusOK, "<html>", "not JSON"},
		"no version": {http.StatusOK, `{"name":"x"}`, "no version"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := registry(t, tc.code, tc.body).Latest(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLatestUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	if _, err := npmreg.New(srv.URL, "1.2.3").Latest(context.Background()); err == nil {
		t.Fatal("want an error for a closed server")
	}
	if _, err := npmreg.New("::", "1.2.3").Latest(context.Background()); err == nil {
		t.Fatal("want an error for a bad base URL")
	}
}

func TestDefaultBase(t *testing.T) {
	if npmreg.DefaultBase != "https://registry.npmjs.org" {
		t.Fatal("default registry changed")
	}
	if npmreg.New("", "x") == nil {
		t.Fatal("nil registry")
	}
}
