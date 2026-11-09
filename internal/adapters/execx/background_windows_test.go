package execx

import (
	"context"
	"syscall"
	"testing"
)

func TestBackgroundHidesTheWindow(t *testing.T) {
	t.Parallel()
	cmd := Command(context.Background(), "git", "version")
	a := cmd.SysProcAttr
	if a == nil || !a.HideWindow || a.CreationFlags&createNoWindow == 0 {
		t.Fatalf("SysProcAttr = %+v, want a hidden window", a)
	}
	if out, err := cmd.Output(); err != nil || len(out) == 0 {
		t.Fatalf("a background program still runs and answers: %q, %v", out, err)
	}
}

func TestBackgroundKeepsWhatTheCommandAlreadySet(t *testing.T) {
	t.Parallel()
	cmd := Command(context.Background(), "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /c exit 0", CreationFlags: 0x200}
	a := Background(cmd).SysProcAttr
	if a.CmdLine != "cmd.exe /c exit 0" || a.CreationFlags&0x200 == 0 || a.CreationFlags&createNoWindow == 0 {
		t.Fatalf("SysProcAttr = %+v, want CmdLine and the old flag kept", a)
	}
}
