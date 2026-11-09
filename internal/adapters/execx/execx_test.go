package execx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelperProcess is not a test: Run's tests start the test binary with
// EXECX_HELPER set, and it prints the given output (or its folder, or a
// variable) and exits with the given code.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("EXECX_HELPER") != "1" {
		return
	}
	switch os.Getenv("EXECX_SHOW") {
	case "dir":
		wd, _ := os.Getwd()
		fmt.Print(wd)
	case "var":
		fmt.Print(os.Getenv("EXECX_VAR"))
	default:
		fmt.Print(os.Getenv("EXECX_OUT"))
	}
	if os.Getenv("EXECX_FAIL") == "1" {
		os.Exit(3)
	}
	os.Exit(0)
}

var self = []string{"-test.run=^TestHelperProcess$"}

func helper(t *testing.T, out string, fail bool) {
	t.Setenv("EXECX_HELPER", "1")
	t.Setenv("EXECX_OUT", out)
	t.Setenv("EXECX_SHOW", "")
	if fail {
		t.Setenv("EXECX_FAIL", "1")
	} else {
		t.Setenv("EXECX_FAIL", "0")
	}
}

func TestRunReturnsOutput(t *testing.T) {
	helper(t, "hello", false)
	out, err := Run(context.Background(), "", nil, os.Args[0], self...)
	if err != nil {
		t.Fatalf("a command that succeeds: %v", err)
	}
	if string(out) != "hello" {
		t.Errorf("output %q, want hello", out)
	}
}

func TestRunInFolder(t *testing.T) {
	helper(t, "", false)
	t.Setenv("EXECX_SHOW", "dir")
	dir := t.TempDir()
	out, err := Run(context.Background(), dir, nil, os.Args[0], self...)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(string(out))
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("ran in %q, want %q", out, dir)
	}
}

func TestRunEnvOverridesInherited(t *testing.T) {
	helper(t, "", false)
	t.Setenv("EXECX_SHOW", "var")
	t.Setenv("EXECX_VAR", "inherited")
	if out, _ := Run(context.Background(), "", nil, os.Args[0], self...); string(out) != "inherited" {
		t.Errorf("no env: got %q, want the inherited value", out)
	}
	if out, _ := Run(context.Background(), "", []string{"EXECX_VAR=given"}, os.Args[0], self...); string(out) != "given" {
		t.Errorf("env: got %q, want the given value", out)
	}
}

func TestRunFailureCarriesExitErrorAndOutput(t *testing.T) {
	helper(t, "  \x1b[31mwent wrong\x1b[0m\n", true)
	out, err := Run(context.Background(), "", nil, os.Args[0], self...)
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want the exit error in the chain, got %v", err)
	}
	if !strings.HasSuffix(err.Error(), ": went wrong") || !strings.HasPrefix(err.Error(), os.Args[0]+": ") {
		t.Errorf("error should be name: exit: trimmed output without colour: %v", err)
	}
	if string(out) != "  \x1b[31mwent wrong\x1b[0m\n" {
		t.Errorf("the raw output should still be returned, got %q", out)
	}
}

func TestRunCutsLongOutput(t *testing.T) {
	helper(t, strings.Repeat("x", maxOutput+500), true)
	_, err := Run(context.Background(), "", nil, os.Args[0], self...)
	if err == nil || !strings.HasSuffix(err.Error(), ": "+strings.Repeat("x", maxOutput)+"…") {
		t.Errorf("output in the error should be cut to %d bytes and marked: %v", maxOutput, err)
	}
}

func TestRunMissingCommand(t *testing.T) {
	if _, err := Run(context.Background(), "", nil, "ccswitch-no-such-command"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("want exec.ErrNotFound, got %v", err)
	}
}

func TestRunHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	helper(t, "", false)
	if _, err := Run(ctx, "", nil, os.Args[0], self...); err == nil {
		t.Fatal("a cancelled context should stop the command")
	}
}

func TestRunInteractive(t *testing.T) {
	helper(t, "", false)
	if err := RunInteractive(context.Background(), []string{"EXECX_OUT="}, os.Args[0], self...); err != nil {
		t.Fatalf("success: %v", err)
	}
	t.Setenv("EXECX_FAIL", "1")
	var exit *exec.ExitError
	if err := RunInteractive(context.Background(), nil, os.Args[0], self...); !errors.As(err, &exit) {
		t.Fatalf("failure: want the exit error, got %v", err)
	}
}

func TestText(t *testing.T) {
	if got := Text([]byte("\x1b[1m2.1.300 (Claude Code)\x1b[0m\n")); got != "2.1.300 (Claude Code)" {
		t.Errorf("got %q", got)
	}
}
