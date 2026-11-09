package keychainitem_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/keychainitem"
)

// exitError is what a fake runner returns for a failed `security`: like
// *exec.ExitError it has an exit code.
type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitError) ExitCode() int { return int(e) }

// fake records the calls and answers with out and err.
type fake struct {
	calls [][]string
	out   string
	err   error
}

func (f *fake) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte(f.out), f.err
}

func item(f *fake) keychainitem.Item {
	return keychainitem.Item{Service: "svc", Account: "acct", Run: f.run}
}

func TestGet(t *testing.T) {
	for name, c := range map[string]struct{ out, want string }{
		"plain":           {`{"a":1}` + "\n", `{"a":1}`},
		"hex for unicode": {hex.EncodeToString([]byte(`{"n":"Señal"}`)) + "\n", `{"n":"Señal"}`},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fake{out: c.out}
			got, err := item(f).Get()
			if err != nil || string(got) != c.want {
				t.Fatalf("Get %q, %v; want %q", got, err, c.want)
			}
			want := "find-generic-password -s svc -a acct -w"
			if len(f.calls) != 1 || strings.Join(f.calls[0], " ") != want {
				t.Fatalf("calls %q, want %q", f.calls, want)
			}
		})
	}
}

func TestGetNotFound(t *testing.T) {
	f := &fake{err: fmt.Errorf("security: %w", exitError(44))}
	_, err := item(f).Get()
	if !errors.Is(err, keychainitem.ErrNotFound) {
		t.Fatalf("err %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), `"svc"`) || !strings.Contains(err.Error(), `"acct"`) {
		t.Fatalf("err %q should name the item", err)
	}
}

func TestGetOtherFailure(t *testing.T) {
	f := &fake{err: exitError(51)}
	_, err := item(f).Get()
	if err == nil || errors.Is(err, keychainitem.ErrNotFound) {
		t.Fatalf("err %v, want a failure that is not ErrNotFound", err)
	}
}

func TestNoAccountRunsNothing(t *testing.T) {
	f := &fake{}
	if _, err := (keychainitem.Item{Service: "svc", Run: f.run}).Get(); err == nil || len(f.calls) != 0 {
		t.Fatalf("err %v, calls %q: want an error and no call", err, f.calls)
	}
}

func TestSetPassesTheDataAsOneArgument(t *testing.T) {
	f := &fake{}
	data := `{"accessToken":"fake-access 'quoted' $HOME"}`
	if err := item(f).Set([]byte(data)); err != nil {
		t.Fatal(err)
	}
	want := []string{"add-generic-password", "-U", "-s", "svc", "-a", "acct", "-w", data}
	if len(f.calls) != 1 || strings.Join(f.calls[0], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("calls %q, want %q", f.calls, want)
	}
}

func TestSetFailureHidesTheData(t *testing.T) {
	f := &fake{err: exitError(1)}
	err := item(f).Set([]byte("fake-secret"))
	if err == nil || strings.Contains(err.Error(), "fake-secret") {
		t.Fatalf("err %v: want an error without the data", err)
	}
}

// attributes is `security find-generic-password` without -w, as macOS
// prints it.
const attributes = `keychain: "/Users/someone/Library/Keychains/login.keychain-db"
version: 512
class: "genp"
attributes:
    0x00000007 <blob>="svc"
    "acct"<blob>="acct"
    "cdat"<timedate>=0x32303235303631323130303030305A00  "20250612100000Z\000"
    "mdat"<timedate>=0x32303236313130393132333435365A00  "20261109123456Z\000"
    "svce"<blob>="svc"
`

func TestModTime(t *testing.T) {
	f := &fake{out: attributes}
	got, err := item(f).ModTime()
	want := time.Date(2026, 11, 9, 12, 34, 56, 0, time.UTC)
	if err != nil || !got.Equal(want) {
		t.Fatalf("ModTime %v, %v; want %v", got, err, want)
	}
	if strings.Join(f.calls[0], " ") != "find-generic-password -s svc -a acct" {
		t.Fatalf("calls %q", f.calls)
	}
}

func TestModTimeFailures(t *testing.T) {
	for name, f := range map[string]*fake{
		"not found": {err: exitError(44)},
		"no mdat":   {out: "attributes:\n    \"acct\"<blob>=\"acct\"\n"},
		"bad hex":   {out: "    \"mdat\"<timedate>=0xZZ  \"x\"\n"},
		"bad date":  {out: "    \"mdat\"<timedate>=0x3230 \"20\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := item(f).ModTime(); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// The helper process stands in for `security`: Command runs this test
// binary with KEYCHAINITEM_HELPER set.
func TestMain(m *testing.M) {
	switch os.Getenv("KEYCHAINITEM_HELPER") {
	case "ok":
		_, _ = fmt.Fprint(os.Stdout, strings.Join(os.Args[1:], " "))
		fmt.Fprint(os.Stderr, "noise on stderr")
		os.Exit(0)
	case "missing":
		fmt.Fprint(os.Stderr, "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.")
		os.Exit(44)
	}
	os.Exit(m.Run())
}

func TestCommand(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := keychainitem.Command(self)

	t.Setenv("KEYCHAINITEM_HELPER", "ok")
	out, err := run(context.Background(), "find-generic-password", "-w")
	if err != nil || string(out) != "find-generic-password -w" {
		t.Fatalf("out %q, %v: want stdout only", out, err)
	}

	t.Setenv("KEYCHAINITEM_HELPER", "missing")
	_, err = keychainitem.Item{Service: "svc", Account: "acct", Run: run}.Get()
	if !errors.Is(err, keychainitem.ErrNotFound) || !strings.Contains(err.Error(), "could not be found") {
		t.Fatalf("err %v, want ErrNotFound with security's message", err)
	}
}
