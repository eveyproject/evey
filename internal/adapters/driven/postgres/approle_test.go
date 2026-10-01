package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
)

// superuserServer starts a throwaway Postgres whose configured login is the
// superuser, the way both the embedded cluster and the shipped compose run.
func superuserServer(t *testing.T) (dsn func(user, password, db string) string) {
	t.Helper()
	testhelpers.SkipIfNoDocker(t)
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Cmd:          []string{"-c", "fsync=off"},
			Env:          map[string]string{"POSTGRES_USER": "evey", "POSTGRES_PASSWORD": "admin-pw", "POSTGRES_DB": "evey"},
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			).WithStartupTimeoutDefault(90 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5432")
	return func(user, password, db string) string {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port.Port(), user, password, db)
	}
}

func openPing(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var pingErr error
	for range 30 {
		if pingErr = db.Ping(); pingErr == nil {
			return db
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("ping: %v", pingErr)
	return nil
}

// adminOwnedCount counts what the admin login still owns in the app's schemas.
func adminOwnedCount(t *testing.T, db *sql.DB, admin string) int {
	t.Helper()
	var n int
	err := db.QueryRow(`
		SELECT (SELECT count(*) FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace
		        WHERE s.nspname = 'public' AND c.relowner = $1::regrole
		          AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e'))
		     + (SELECT count(*) FROM pg_proc p JOIN pg_namespace s ON s.oid = p.pronamespace
		        WHERE s.nspname = 'public' AND p.proowner = $1::regrole
		          AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e'))
		     + (SELECT count(*) FROM pg_database WHERE datname = current_database() AND datdba = $1::regrole)`, admin).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// An existing deployment (every object owned by the superuser login) is handed
// to a role that cannot reach the host: no COPY PROGRAM, no file read, no way
// back to the superuser, while the app and its migrations keep working.
func TestEnsureAppRole_ExistingDatabaseIsHandedToAnUnprivilegedRole(t *testing.T) {
	dsn := superuserServer(t)
	ctx := context.Background()
	admin := openPing(t, dsn("evey", "admin-pw", "evey"))
	if err := postgres.RunMigrations(admin, postgres.BackupOpts{}); err != nil {
		t.Fatalf("migrations as the superuser (today's deployments): %v", err)
	}
	if n := adminOwnedCount(t, admin, "evey"); n == 0 {
		t.Fatal("precondition: the superuser should own the schema before the split")
	}
	// A role of that name widened by hand must be narrowed back, not trusted.
	if _, err := admin.Exec(`CREATE ROLE evey_app SUPERUSER CREATEROLE; GRANT evey, pg_execute_server_program TO evey_app`); err != nil {
		t.Fatal(err)
	}

	pw, applied, err := postgres.EnsureAppRole(ctx, admin, "evey_app")
	if err != nil || !applied || pw == "" {
		t.Fatalf("EnsureAppRole: applied=%v err=%v", applied, err)
	}
	if n := adminOwnedCount(t, admin, "evey"); n != 0 {
		t.Fatalf("the superuser still owns %d objects after the split", n)
	}
	var verifier string
	if err = admin.QueryRow(`SELECT rolpassword FROM pg_authid WHERE rolname = 'evey_app'`).Scan(&verifier); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(verifier, "SCRAM-SHA-256$4096:") || strings.Contains(verifier, pw) {
		t.Fatalf("password must be sent as a SCRAM verifier, got %q", verifier)
	}

	app := openPing(t, dsn("evey_app", pw, "evey"))
	var super bool
	if err = app.QueryRow(`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&super); err != nil || super {
		t.Fatalf("app role superuser=%v err=%v", super, err)
	}
	if err = postgres.RunMigrations(app, postgres.BackupOpts{}); err != nil {
		t.Fatalf("migrations as the app role: %v", err)
	}
	if _, err = app.Exec(`CREATE TABLE zz_future_migration (id int primary key); DROP TABLE zz_future_migration`); err != nil {
		t.Fatalf("a future migration's DDL as the app role: %v", err)
	}
	if _, err = app.Exec(`INSERT INTO notification_settings_global (id) VALUES (1) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("app DML: %v", err)
	}
	for name, stmt := range map[string]string{
		"copy program":   `COPY (SELECT 1) TO PROGRAM 'id'`,
		"read file":      `SELECT pg_read_file('/etc/passwd')`,
		"set role admin": `SET ROLE evey`,
		"self promotion": `ALTER ROLE evey_app SUPERUSER`,
		"new role":       `CREATE ROLE zz_attacker LOGIN SUPERUSER`,
	} {
		if _, err = app.Exec(stmt); err == nil {
			t.Errorf("%s: the app role was allowed to run %q", name, stmt)
		}
	}

	// An admin command next to the live server hands over what it created
	// without rotating: the server's pool keeps logging in.
	if _, err = admin.Exec(`CREATE TABLE zz_admin_made (id int)`); err != nil {
		t.Fatal(err)
	}
	if applied, err = postgres.HandOverToAppRole(ctx, admin, "evey_app"); err != nil || !applied {
		t.Fatalf("HandOverToAppRole: applied=%v err=%v", applied, err)
	}
	if n := adminOwnedCount(t, admin, "evey"); n != 0 {
		t.Fatalf("hand-over left %d objects with the superuser", n)
	}
	openPing(t, dsn("evey_app", pw, "evey"))

	// Every boot rotates the password: the previous one stops working.
	pw2, applied, err := postgres.EnsureAppRole(ctx, admin, "evey_app")
	if err != nil || !applied || pw2 == pw {
		t.Fatalf("second EnsureAppRole: applied=%v err=%v same=%v", applied, err, pw2 == pw)
	}
	stale, _ := sql.Open("postgres", dsn("evey_app", pw, "evey"))
	defer stale.Close()
	if err = stale.Ping(); err == nil {
		t.Fatal("the previous boot's password still logs in")
	}
	openPing(t, dsn("evey_app", pw2, "evey"))
}

// A fresh install migrates as the app role from the first boot, so the
// superuser never owns anything (pg_trgm is a trusted extension).
func TestEnsureAppRole_FreshDatabaseMigratesAsTheAppRole(t *testing.T) {
	dsn := superuserServer(t)
	ctx := context.Background()
	admin := openPing(t, dsn("evey", "admin-pw", "evey"))
	pw, applied, err := postgres.EnsureAppRole(ctx, admin, "evey_app")
	if err != nil || !applied {
		t.Fatalf("EnsureAppRole: applied=%v err=%v", applied, err)
	}
	app := openPing(t, dsn("evey_app", pw, "evey"))
	if err := postgres.RunMigrations(app, postgres.BackupOpts{}); err != nil {
		t.Fatalf("fresh migrations as the app role: %v", err)
	}
	if n := adminOwnedCount(t, admin, "evey"); n != 0 {
		t.Fatalf("the superuser owns %d objects on a fresh install", n)
	}
}

// An operator who already configured an unprivileged login keeps it as is.
func TestEnsureAppRole_NonSuperuserLoginIsLeftAlone(t *testing.T) {
	dsn := superuserServer(t)
	ctx := context.Background()
	admin := openPing(t, dsn("evey", "admin-pw", "evey"))
	if _, err := admin.Exec(`CREATE ROLE operator_app LOGIN PASSWORD 'op-pw'`); err != nil {
		t.Fatal(err)
	}
	op := openPing(t, dsn("operator_app", "op-pw", "evey"))
	_, applied, err := postgres.EnsureAppRole(ctx, op, "evey_app")
	if err != nil || applied {
		t.Fatalf("non-superuser login: applied=%v err=%v, want untouched", applied, err)
	}
}

func TestEnsureAppRole_RejectsUnsafeOrSelfRoleNames(t *testing.T) {
	dsn := superuserServer(t)
	admin := openPing(t, dsn("evey", "admin-pw", "evey"))
	for _, name := range []string{"", "evey", `evey"; DROP TABLE x; --`, "Evey App", strings.Repeat("a", 64)} {
		if _, _, err := postgres.EnsureAppRole(context.Background(), admin, name); err == nil {
			t.Errorf("role name %q accepted", name)
		}
	}
}
