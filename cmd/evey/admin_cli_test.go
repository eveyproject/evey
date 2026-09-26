package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The completion script and the container's admin shell each carry their own
// list of subcommands; three of them (ladder-bench, ladder-probe, route-bench)
// were once missing from both. The table is the reference.

func TestBashCompletionListsEveryAdminSubcommand(t *testing.T) {
	subs := shellWordList(t, bashCompletion, `subs="`)
	for _, c := range adminCommands {
		if !subs[c.name] {
			t.Errorf("evey-completion.bash: subcommand %q missing from subs=", c.name)
		}
	}
}

func TestAdminShellListsEveryAdminSubcommand(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docker", "evey-shell.sh"))
	if err != nil {
		t.Fatalf("read evey-shell.sh: %v", err)
	}
	verbs := shellWordList(t, string(raw), `_EVEY_VERBS='`)
	for _, c := range adminCommands {
		if !verbs[c.name] {
			t.Errorf("evey-shell.sh: subcommand %q missing from _EVEY_VERBS", c.name)
		}
	}
}

// Every flag a subcommand declares must be offered by the completion script.
var flagDecl = regexp.MustCompile(`\.(?:String|Int|Bool|Float64|Duration)\("([a-z-]+)"`)

func TestBashCompletionOffersEveryDeclaredFlag(t *testing.T) {
	files, err := filepath.Glob("admin*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range flagDecl.FindAllStringSubmatch(string(src), -1) {
			if !strings.Contains(bashCompletion, "--"+m[1]) {
				t.Errorf("%s declares --%s but evey-completion.bash never offers it", f, m[1])
			}
		}
	}
}

func TestAdminUsageAndManualCoverEveryAdminSubcommand(t *testing.T) {
	var usage, roff, text bytes.Buffer
	printAdminUsage(&usage)
	renderRoff(&roff, manual())
	renderText(&text, manual())
	for _, c := range adminCommands {
		if !strings.Contains(usage.String(), "\n  "+c.name) {
			t.Errorf("admin usage lacks %q", c.name)
		}
		if !strings.Contains(roff.String(), roffEscape("evey admin "+c.name)) {
			t.Errorf("roff manual lacks %q", c.name)
		}
		if !strings.Contains(text.String(), "evey admin "+c.name) {
			t.Errorf("text manual lacks %q", c.name)
		}
	}
	if strings.Contains(roff.String(), "\n.\n") {
		t.Error("roff manual has a bare dot line")
	}
}

func TestRunCLIRoutesOneShotCommands(t *testing.T) {
	stdout := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = devnull
	t.Cleanup(func() { os.Stdout = stdout; _ = devnull.Close() })
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"version"}, {"man"}, {"man", "--text"}, {"completion", "bash"}} {
		if handled, err := runCLI(args); !handled || err != nil {
			t.Errorf("runCLI(%v) = handled %v, err %v", args, handled, err)
		}
	}
	if handled, _ := runCLI(nil); handled {
		t.Error("no argument must boot the server")
	}
	for _, args := range [][]string{{"serve"}, {"--foo"}, {"completion", "zsh"}, {"man", "x"}} {
		if handled, err := runCLI(args); !handled || err == nil {
			t.Errorf("runCLI(%v) must be handled and fail, got handled %v err %v", args, handled, err)
		}
	}
}

func TestAdminHelpNeverRunsTheSubcommand(t *testing.T) {
	stdout := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = devnull
	t.Cleanup(func() { os.Stdout = stdout; _ = devnull.Close() })
	// Handlers without a FlagSet used to run on --help (rollback, list-users)
	// or fail on a missing positional (restore-backup); they need no DB here.
	for _, c := range adminCommands {
		if !c.noFlags {
			continue
		}
		if err := runAdmin([]string{c.name, "--help"}); err != nil {
			t.Errorf("evey admin %s --help: %v", c.name, err)
		}
	}
	if err := runAdmin([]string{"help"}); err != nil {
		t.Errorf("evey admin help: %v", err)
	}
	if err := runAdmin([]string{"no-such-command"}); err == nil {
		t.Error("unknown subcommand must fail")
	}
}

func TestWrapText(t *testing.T) {
	got := wrapText("aaa bbb ccc ddd", 8, "  ")
	if want := "  aaa\n  bbb\n  ccc\n  ddd"; got != want {
		t.Errorf("wrapText = %q, want %q", got, want)
	}
}

// shellWordList parses `<marker>word word \<newline>word'` into a set.
func shellWordList(t *testing.T, src, marker string) map[string]bool {
	t.Helper()
	i := strings.Index(src, marker)
	if i < 0 {
		t.Fatalf("marker %q not found", marker)
	}
	rest := src[i+len(marker):]
	end := strings.IndexAny(rest, `"'`)
	if end < 0 {
		t.Fatalf("unterminated list after %q", marker)
	}
	set := map[string]bool{}
	for w := range strings.FieldsSeq(strings.ReplaceAll(rest[:end], "\\\n", " ")) {
		set[w] = true
	}
	return set
}
