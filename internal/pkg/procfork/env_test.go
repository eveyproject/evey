package procfork

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// The child sees what the hardware paths and the toolchain read, and none of
// the server's own secrets.
func TestChildEnv_KeepsToolchainAndHardwareDropsSecrets(t *testing.T) {
	got := childEnv([]string{
		"PATH=/usr/bin",
		"HOME=/var/lib/evey",
		"LIBVA_DRIVER_NAME=iHD",
		"NVIDIA_VISIBLE_DEVICES=all",
		"LC_ALL=C.UTF-8",
		"EVEY_DB_DSN=postgres://evey:hunter2@db/evey",
		"EVEY_OPENSUBTITLES_PASSWORD=hunter2",
		"EVEY_TOKEN=abc",
		"AWS_SECRET_ACCESS_KEY=xyz",
		"malformed",
	})
	for _, want := range []string{"PATH=/usr/bin", "HOME=/var/lib/evey", "LIBVA_DRIVER_NAME=iHD", "NVIDIA_VISIBLE_DEVICES=all", "LC_ALL=C.UTF-8"} {
		if !slices.Contains(got, want) {
			t.Errorf("%q must reach the child, got %v", want, got)
		}
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "EVEY_") || strings.HasPrefix(kv, "AWS_") || kv == "malformed" {
			t.Errorf("%q must not reach the child", kv)
		}
	}
}

// What a spawned child actually reads from its environment: the server's
// variables are not there.
func TestCommand_ChildDoesNotInheritTheServerEnvironment(t *testing.T) {
	t.Setenv("EVEY_TEST_SECRET_NOT_FOR_CHILDREN", "hunter2")
	cmd, finish, err := Command(context.Background(), Options{Class: Probe, NoMemCap: true}, "env")
	if err != nil {
		t.Skipf("spawn refused here: %v", err)
	}
	out, runErr := cmd.Output()
	_ = finish(runErr, nil)
	if runErr != nil {
		t.Skipf("env not runnable here: %v", runErr)
	}
	if strings.Contains(string(out), "EVEY_TEST_SECRET_NOT_FOR_CHILDREN") {
		t.Fatalf("the child read the server's variable:\n%s", out)
	}
	if !strings.Contains(string(out), "PATH=") {
		t.Fatalf("the child must still have a PATH:\n%s", out)
	}
}
