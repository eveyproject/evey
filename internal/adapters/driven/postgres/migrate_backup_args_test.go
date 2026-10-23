package postgres

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pre-migration dump is replayed onto a populated database by
// `evey admin restore-backup`; without --clean --if-exists the restore dies on
// "relation already exists".
func TestDumpDatabase_PassesCleanIfExists(t *testing.T) {
	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\necho '-- done'\n"
	if err := os.WriteFile(filepath.Join(bin, "pg_dump"), []byte(script), 0o700); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := dumpDatabase(BackupOpts{Dir: t.TempDir(), Host: "h", Port: 5432, User: "u", DBName: "d"}, 7)
	if err != nil {
		t.Fatalf("dumpDatabase: %v", err)
	}
	raw, err := os.ReadFile(argsFile) //nolint:gosec // path built from t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, want := range []string{"--clean", "--if-exists", "--no-owner", "--no-privileges"} {
		found := false
		for _, a := range args {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("pg_dump argv missing %s: %v", want, args)
		}
	}
}
