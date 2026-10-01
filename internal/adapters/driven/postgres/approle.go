package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/lib/pq"
)

// The server logs in with whatever the operator configured, which on both the
// embedded cluster and the shipped compose is the superuser. Any SQL reaching
// such a session (an injection, a forged restore) can COPY ... TO PROGRAM and
// run on the host. EnsureAppRole splits the two: the superuser login only
// creates an unprivileged role, hands it the schema, and is kept for what
// truly needs it (the restore disables system FK triggers); the app pool and
// the migrations run as that role.

var appRoleName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// EnsureAppRole makes role an unprivileged login that owns the current
// database and every object the superuser login owns in it, and sets it a
// fresh random password returned to the caller (kept in memory, rotated every
// boot). applied is false, with nothing changed, when admin is not a
// superuser: the operator already runs least-privilege.
func EnsureAppRole(ctx context.Context, admin *sql.DB, role string) (password string, applied bool, err error) {
	return ensureAppRole(ctx, admin, role, true)
}

// HandOverToAppRole does EnsureAppRole's work without touching the password,
// for `evey admin` commands run next to a live server: rotating it there would
// lock the server's pool out of its next connection.
func HandOverToAppRole(ctx context.Context, admin *sql.DB, role string) (applied bool, err error) {
	_, applied, err = ensureAppRole(ctx, admin, role, false)
	return applied, err
}

func ensureAppRole(ctx context.Context, admin *sql.DB, role string, rotate bool) (password string, applied bool, err error) {
	if !appRoleName.MatchString(role) {
		return "", false, fmt.Errorf("app role name %q: want lowercase letters, digits and _", role)
	}
	var self string
	var super bool
	if err = admin.QueryRowContext(ctx,
		`SELECT current_user, rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&self, &super); err != nil {
		return "", false, fmt.Errorf("read login role: %w", err)
	}
	if !super {
		return "", false, nil
	}
	if self == role {
		return "", false, errors.New("app role must differ from the configured superuser login")
	}

	setPassword := ""
	if rotate {
		var verifier string
		if password, verifier, err = newSCRAMPassword(); err != nil {
			return "", false, err
		}
		setPassword = " PASSWORD " + pq.QuoteLiteral(verifier)
	}
	ident := pq.QuoteIdentifier(role)
	tx, err := admin.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists); err != nil {
		return "", false, fmt.Errorf("look up app role: %w", err)
	}
	if !exists {
		if _, err = tx.ExecContext(ctx, "CREATE ROLE "+ident); err != nil {
			return "", false, fmt.Errorf("create app role: %w", err)
		}
	}
	// Re-asserted on every boot: a role someone widened by hand is narrowed
	// back. The verifier, not the password, travels: a log_statement=ddl
	// server never writes the secret down.
	if _, err = tx.ExecContext(ctx, "ALTER ROLE "+ident+
		" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT"+setPassword); err != nil {
		return "", false, fmt.Errorf("narrow app role: %w", err)
	}
	if err = revokeMemberships(ctx, tx, role, ident); err != nil {
		return "", false, err
	}
	if _, err = tx.ExecContext(ctx, transferOwnershipSQL(ident)); err != nil {
		return "", false, fmt.Errorf("hand the schema to %s: %w", role, err)
	}
	if err = tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit: %w", err)
	}
	if rotate {
		slog.InfoContext(ctx, "database: app runs as an unprivileged role", slog.String("role", role), slog.String("admin", self))
	}
	return password, true, nil
}

// revokeMemberships drops every role the app role inherits from: membership in
// the superuser or pg_execute_server_program would undo the split.
func revokeMemberships(ctx context.Context, tx *sql.Tx, role, ident string) error {
	parents, err := memberships(ctx, tx, role)
	if err != nil {
		return err
	}
	for _, p := range parents {
		if _, err := tx.ExecContext(ctx, "REVOKE "+pq.QuoteIdentifier(p)+" FROM "+ident); err != nil {
			return fmt.Errorf("revoke %s from app role: %w", p, err)
		}
	}
	return nil
}

// memberships lists them first: the REVOKEs cannot run on the transaction's
// connection while its rows are still open.
func memberships(ctx context.Context, tx *sql.Tx, role string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT r.rolname FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.roleid
		 WHERE m.member = (SELECT oid FROM pg_roles WHERE rolname = $1)`, role)
	if err != nil {
		return nil, fmt.Errorf("list app role memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var parents []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		parents = append(parents, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list app role memberships: %w", err)
	}
	return parents, nil
}

// transferOwnershipSQL hands the database, its schemas and every relation,
// routine and standalone type the login owns to app. REASSIGN OWNED cannot be
// used: it refuses the bootstrap superuser, which is exactly our login.
// Extension members stay with the extension; owned sequences follow their table.
func transferOwnershipSQL(app string) string {
	return `DO $evey$
DECLARE
	app text := ` + pq.QuoteLiteral(app) + `;
	me oid := (SELECT oid FROM pg_roles WHERE rolname = current_user);
	r record;
BEGIN
	EXECUTE format('ALTER DATABASE %I OWNER TO %s', current_database(), app);
	FOR r IN SELECT n.nspname FROM pg_namespace n
		WHERE n.nspowner = me AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_%'
	LOOP
		EXECUTE format('ALTER SCHEMA %I OWNER TO %s', r.nspname, app);
	END LOOP;
	FOR r IN SELECT c.oid::regclass AS obj, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relowner = me AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_%'
		  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e')
	LOOP
		EXECUTE format('ALTER %s %s OWNER TO %s',
			CASE r.relkind WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' WHEN 'f' THEN 'FOREIGN TABLE' ELSE 'TABLE' END,
			r.obj, app);
	END LOOP;
	FOR r IN SELECT c.oid::regclass AS obj FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relowner = me AND c.relkind = 'S'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_%'
		  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype IN ('e', 'a', 'i'))
	LOOP
		EXECUTE format('ALTER SEQUENCE %s OWNER TO %s', r.obj, app);
	END LOOP;
	FOR r IN SELECT p.oid::regprocedure AS obj FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.proowner = me
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_%'
		  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')
	LOOP
		EXECUTE format('ALTER ROUTINE %s OWNER TO %s', r.obj, app);
	END LOOP;
	FOR r IN SELECT t.oid::regtype AS obj FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE t.typowner = me
		  AND (t.typtype IN ('e', 'd', 'r') OR (t.typtype = 'c' AND (SELECT relkind FROM pg_class WHERE oid = t.typrelid) = 'c'))
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_%'
		  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_type'::regclass AND d.objid = t.oid AND d.deptype = 'e')
	LOOP
		EXECUTE format('ALTER TYPE %s OWNER TO %s', r.obj, app);
	END LOOP;
END
$evey$`
}

// newSCRAMPassword returns a random password and its SCRAM-SHA-256 verifier in
// Postgres' stored format (RFC 5802/7677, what psql's \password sends).
func newSCRAMPassword() (password, verifier string, err error) {
	raw := make([]byte, 32)
	salt := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("app role password: %w", err)
	}
	if _, err = rand.Read(salt); err != nil {
		return "", "", fmt.Errorf("app role salt: %w", err)
	}
	password = hex.EncodeToString(raw)
	const iterations = 4096
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return "", "", fmt.Errorf("app role verifier: %w", err)
	}
	mac := func(key []byte, msg string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(msg))
		return h.Sum(nil)
	}
	stored := sha256.Sum256(mac(salted, "Client Key"))
	b64 := base64.StdEncoding.EncodeToString
	verifier = fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", iterations, b64(salt), b64(stored[:]), b64(mac(salted, "Server Key")))
	return password, verifier, nil
}
