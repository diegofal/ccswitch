// Package testkit holds in-memory fakes for every port and the test fixtures
// shared by the domain, core and adapter tests.
package testkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Fixture returns the contents of internal/testkit/testdata/<name>. The path
// comes from this source file, so it works from any package's test.
func Fixture(t testing.TB, name string) []byte {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testkit: cannot locate testdata")
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(self), "testdata", name))
	if err != nil {
		t.Fatalf("testkit: fixture %s: %v", name, err)
	}
	return b
}
