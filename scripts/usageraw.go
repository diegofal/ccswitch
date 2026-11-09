//go:build ignore

// usageraw prints the raw usage response of a vault account, to see the
// shape of a body the parser refuses. Usage: go run scripts/usageraw.go <email>
// The body holds no secret; the token never leaves the process.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/adapters/vault"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: usageraw <email>")
		os.Exit(2)
	}
	dir, err := paths.DataDir()
	if err != nil {
		panic(err)
	}
	v, err := vault.Open(dir)
	if err != nil {
		panic(err)
	}
	accts, err := v.Load()
	if err != nil {
		panic(err)
	}
	for _, a := range accts {
		if a.Email != os.Args[1] {
			continue
		}
		tok, err := a.AccessToken()
		if err != nil {
			panic(err)
		}
		req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("anthropic-beta", "oauth-2025-04-20")
		req.Header.Set("Accept", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			panic(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		fmt.Println(res.Status)
		fmt.Println(string(body))
		return
	}
	fmt.Fprintln(os.Stderr, "not in the vault")
	os.Exit(1)
}
