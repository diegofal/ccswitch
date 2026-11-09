package internal_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/diegofal/ccswitch"

// layer describes one group of packages: which internal packages it may
// import and whether it may import modules outside the standard library.
type layer struct {
	name     string
	match    func(rel string) bool
	internal []string // allowed internal prefixes (relative to the module)
	external []string // allowed external module prefixes; "*" allows any
}

func under(prefixes ...string) func(string) bool {
	return func(rel string) bool {
		for _, p := range prefixes {
			if rel == p || strings.HasPrefix(rel, p+"/") {
				return true
			}
		}
		return false
	}
}

var layers = []layer{
	{name: "domain", match: under("internal/domain")},
	{name: "ports", match: under("internal/ports"), internal: []string{"internal/domain", "internal/ports"}},
	{name: "testkit", match: under("internal/testkit"), internal: []string{"internal/domain", "internal/ports", "internal/testkit"}},
	{name: "core", match: under("internal/core"), internal: []string{"internal/domain", "internal/ports", "internal/core"}},
	{name: "adapters", match: under("internal/adapters"), internal: []string{"internal/domain", "internal/ports", "internal/adapters"}, external: []string{"*"}},
	{name: "front ends", match: under("internal/cli", "internal/tray"), internal: []string{"internal/domain", "internal/core", "internal/cli", "internal/tray"}, external: []string{"*"}},
	{name: "wiring", match: under("cmd"), internal: []string{""}, external: []string{"*"}},
	{name: "arch test", match: func(rel string) bool { return rel == "internal" }},
}

func isStd(imp string) bool {
	first := strings.SplitN(imp, "/", 2)[0]
	return !strings.Contains(first, ".")
}

func allowed(prefixes []string, s string) bool {
	for _, p := range prefixes {
		if p == "*" || p == "" || s == p || strings.HasPrefix(s, p+"/") {
			return true
		}
	}
	return false
}

// check returns the rule violations of one package.
func check(pkg string, imports []string) []string {
	rel := strings.TrimPrefix(strings.TrimPrefix(pkg, module), "/")
	var l *layer
	for i := range layers {
		if layers[i].match(rel) {
			l = &layers[i]
			break
		}
	}
	if l == nil {
		return []string{pkg + ": belongs to no layer; add it to internal/arch_test.go"}
	}
	var bad []string
	for _, imp := range imports {
		switch {
		case isStd(imp):
		case imp == module || strings.HasPrefix(imp, module+"/"):
			target := strings.TrimPrefix(strings.TrimPrefix(imp, module), "/")
			if !allowed(l.internal, target) {
				bad = append(bad, rel+" ("+l.name+") must not import "+target)
			}
		default:
			if !allowed(l.external, imp) {
				bad = append(bad, rel+" ("+l.name+") must not import external "+imp)
			}
		}
	}
	return bad
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestArchitecture(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \",\"}}", "./...")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imps, _ := strings.Cut(strings.TrimSpace(line), "|")
		var imports []string
		if imps != "" {
			imports = strings.Split(imps, ",")
		}
		for _, v := range check(pkg, imports) {
			t.Error(v)
		}
	}
}

func TestArchitectureCatchesViolations(t *testing.T) {
	bad := []struct {
		pkg     string
		imports []string
	}{
		{module + "/internal/domain", []string{module + "/internal/ports"}},
		{module + "/internal/domain", []string{"github.com/some/lib"}},
		{module + "/internal/ports", []string{module + "/internal/core"}},
		{module + "/internal/ports", []string{"github.com/some/lib"}},
		{module + "/internal/testkit", []string{module + "/internal/core"}},
		{module + "/internal/testkit", []string{"github.com/some/lib"}},
		{module + "/internal/core", []string{module + "/internal/adapters/usageapi"}},
		{module + "/internal/core", []string{"github.com/some/lib"}},
		{module + "/internal/adapters/usageapi", []string{module + "/internal/core"}},
		{module + "/internal/cli", []string{module + "/internal/adapters/loginfile"}},
		{module + "/internal/cli", []string{module + "/internal/ports"}},
		{module + "/internal/tray", []string{module + "/internal/testkit"}},
		{module + "/internal/newthing", nil},
	}
	for _, c := range bad {
		if len(check(c.pkg, c.imports)) == 0 {
			t.Errorf("%s importing %v should violate the rules", c.pkg, c.imports)
		}
	}
	ok := []struct {
		pkg     string
		imports []string
	}{
		{module + "/internal/domain", []string{"encoding/json", "time"}},
		{module + "/internal/ports", []string{module + "/internal/domain", "context"}},
		{module + "/internal/testkit", []string{module + "/internal/domain", module + "/internal/ports"}},
		{module + "/internal/core", []string{module + "/internal/domain", module + "/internal/ports", module + "/internal/core/state"}},
		{module + "/internal/adapters/vault/dpapi", []string{"golang.org/x/sys/windows", module + "/internal/adapters/paths", module + "/internal/ports"}},
		{module + "/internal/tray", []string{"github.com/wailsapp/wails/v3/pkg/application", module + "/internal/core", module + "/internal/domain"}},
		{module + "/cmd/ccswitch", []string{module + "/internal/adapters/loginfile", module + "/internal/core", module + "/internal/cli"}},
	}
	for _, c := range ok {
		if v := check(c.pkg, c.imports); len(v) != 0 {
			t.Errorf("%s should be allowed: %v", c.pkg, v)
		}
	}
}

// homeLookups are the ways to find the user's home or config folders. Only
// internal/adapters/paths may use them: every other package gets its folders
// from there, so CCSWITCH_DATA_DIR and CCSWITCH_CLAUDE_DIR redirect all of
// them and no test can reach the real ~/.claude or vault.
var homeLookups = []string{`os.UserHomeDir`, `os.Getenv("HOME")`, `"USERPROFILE"`, `"APPDATA"`, `"XDG_`}

// homeLookup returns the first home lookup in Go source, or "".
func homeLookup(src string) string {
	for _, s := range homeLookups {
		if strings.Contains(src, s) {
			return s
		}
	}
	return ""
}

// goFiles calls fn with the slash path (relative to the repo) and contents of
// every Go file under the given top-level folders that exist.
func goFiles(t *testing.T, dirs []string, fn func(rel, src string)) {
	t.Helper()
	root := repoRoot(t)
	for _, dir := range dirs {
		top := filepath.Join(root, dir)
		if _, err := os.Stat(top); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(top, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fn(filepath.ToSlash(rel), string(src))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestHomeFoldersOnlyFromPaths(t *testing.T) {
	goFiles(t, []string{"internal", "cmd"}, func(rel, src string) {
		if rel == "internal/arch_test.go" || strings.HasPrefix(rel, "internal/adapters/paths/") {
			return
		}
		if s := homeLookup(src); s != "" {
			t.Errorf("%s uses %s; get folders from internal/adapters/paths", rel, s)
		}
	})
	for _, src := range []string{`h, _ := os.UserHomeDir()`, `os.Getenv("HOME")`, `os.Getenv("USERPROFILE")`, `os.Getenv("APPDATA")`, `os.Getenv("XDG_DATA_HOME")`} {
		if homeLookup(src) == "" {
			t.Errorf("homeLookup must catch %s", src)
		}
	}
	if s := homeLookup(`dir := paths.DataDir()`); s != "" {
		t.Errorf("homeLookup flagged %q", s)
	}
}

// startsProgram reports whether Go source calls exec.Command or
// exec.CommandContext.
func startsProgram(src string) bool {
	return strings.Contains(src, "exec.Command(") || strings.Contains(src, "exec.CommandContext(")
}

// Every program start goes through internal/adapters/execx, which hides the
// console window on Windows: the tray has no console, so a bare exec.Command
// flashes a `claude` window that steals focus. Tests may start programs
// directly (the fake claude binary, go list here).
func TestProgramsStartThroughExecx(t *testing.T) {
	goFiles(t, []string{"internal", "cmd"}, func(rel, src string) {
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/adapters/execx/") {
			return
		}
		if startsProgram(src) {
			t.Errorf("%s starts a program with os/exec; use execx so it opens no window", rel)
		}
	})
	if !startsProgram(`cmd := exec.CommandContext(ctx, "claude")`) || startsProgram(`cmd := execx.Command(ctx, "claude")`) {
		t.Error("startsProgram must tell exec.Command from execx.Command")
	}
}
