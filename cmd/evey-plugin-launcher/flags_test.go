package main

import (
	"flag"
	"io"
	"testing"
)

// The flag types behind -ro/-rw/-dev and -net-connect: repeatable paths keep
// their order and drop blanks; a port must be a real TCP port (1-65535) so a
// mistyped grant refuses the launch instead of silently opening nothing or
// wrapping around to another port.
func TestLauncherFlagTypes(t *testing.T) {
	var paths stringList
	for _, v := range []string{"/usr", "", "/data"} {
		if err := paths.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	if got := paths.String(); got != "/usr,/data" {
		t.Errorf("stringList = %q", got)
	}

	var ports portList
	for _, v := range []string{"9091", "443"} {
		if err := ports.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	if got := ports.String(); got != "9091,443" {
		t.Errorf("portList = %q", got)
	}
	for _, bad := range []string{"0", "65536", "-1", "http", "80.5", ""} {
		if err := ports.Set(bad); err == nil {
			t.Errorf("port %q must be refused", bad)
		}
	}
	if len(ports) != 2 {
		t.Errorf("a refused port must not be appended: %v", ports)
	}
}

// A bad flag is a usage error (2), before any confinement is attempted.
func TestRun_BadFlagIsUsageError(t *testing.T) {
	flag.CommandLine.SetOutput(io.Discard)
	if code := run([]string{"-net-connect", "nope", "--", "/bin/true"}); code != 2 {
		t.Errorf("invalid port should return 2, got %d", code)
	}
	if code := run([]string{"-no-such-flag", "--", "/bin/true"}); code != 2 {
		t.Errorf("unknown flag should return 2, got %d", code)
	}
}
