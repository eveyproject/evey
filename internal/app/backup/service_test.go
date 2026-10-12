package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	backupmodel "github.com/eveyproject/evey/internal/domain/backup"
	"github.com/eveyproject/evey/internal/domain/backup/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type fakeEventCollector struct {
	events []shared.Event
}

func (f *fakeEventCollector) Publish(_ context.Context, event shared.Event) error {
	f.events = append(f.events, event)
	return nil
}
func (f *fakeEventCollector) Subscribe(string, func(context.Context, shared.Event)) string {
	return ""
}
func (f *fakeEventCollector) Unsubscribe(string) {}

type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func setup() (*Service, *fakes.FakeBackupStore, *fakes.FakeDatabaseDumper) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "0.1.0")
	return svc, store, dumper
}

// buildTarGz builds a tar.gz archive identical to what BackupStore.Create produces.
func buildTarGz(t *testing.T, manifest *backupmodel.BackupManifest, dump []byte, signature []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeTar(t, tw, "manifest.json", manifestBytes)
	writeTar(t, tw, "dump.sql", dump)
	if signature != nil {
		writeTar(t, tw, "signature", signature)
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func writeTar(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0640}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}

func makeManifest(dump []byte) *backupmodel.BackupManifest {
	hash := sha256.Sum256(dump)
	return &backupmodel.BackupManifest{
		ID:         "bk-test",
		CreatedAt:  time.Now().UTC(),
		Version:    "1.0.0",
		Tables:     BackupTables,
		DumpSHA256: hex.EncodeToString(hash[:]),
	}
}

func TestBackupTables_ContainsRequiredTables(t *testing.T) {
	required := []string{"api_keys", "push_subscriptions"}
	tableSet := make(map[string]bool, len(BackupTables))
	for _, tbl := range BackupTables {
		tableSet[tbl] = true
	}
	for _, want := range required {
		if !tableSet[want] {
			t.Errorf("BackupTables missing required table %q", want)
		}
	}
}

func TestCreateBackup(t *testing.T) {
	svc, store, _ := setup()
	ctx := context.Background()

	m, err := svc.CreateBackup(ctx)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if m.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if m.Version != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %s", m.Version)
	}
	if len(m.Tables) == 0 {
		t.Fatal("expected tables list")
	}
	if m.DumpSHA256 == "" {
		t.Fatal("expected DumpSHA256 to be set")
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(list))
	}
}

func TestListBackups(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()
	svc.CreateBackup(ctx)
	svc.CreateBackup(ctx)

	list, err := svc.ListBackups(ctx)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(list))
	}
}

func TestGetBackup(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	m, _ := svc.CreateBackup(ctx)
	got, rc, err := svc.GetBackup(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	defer rc.Close()
	if got.ID != m.ID {
		t.Fatalf("expected ID %s, got %s", m.ID, got.ID)
	}
	data, _ := io.ReadAll(rc)
	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}
}

func TestGetBackupNotFound(t *testing.T) {
	svc, _, _ := setup()
	_, _, err := svc.GetBackup(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestDeleteBackup(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	m, _ := svc.CreateBackup(ctx)
	if err := svc.DeleteBackup(ctx, m.ID); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	list, _ := svc.ListBackups(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0 backups after delete, got %d", len(list))
	}
}

func TestDeleteBackupNotFound(t *testing.T) {
	svc, _, _ := setup()
	err := svc.DeleteBackup(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestCleanupOldBackups_DeletesOld(t *testing.T) {
	svc, store, _ := setup()
	ctx := context.Background()

	old, _ := svc.CreateBackup(ctx)
	recent, _ := svc.CreateBackup(ctx)
	store.SetCreatedAt(old.ID, time.Now().UTC().AddDate(0, 0, -31))

	if err := svc.CleanupOldBackups(ctx, 30); err != nil {
		t.Fatalf("CleanupOldBackups: %v", err)
	}
	list, _ := svc.ListBackups(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1 backup after cleanup, got %d", len(list))
	}
	if list[0].ID != recent.ID {
		t.Errorf("expected recent backup %s to survive, got %s", recent.ID, list[0].ID)
	}
}

func TestCleanupOldBackups_KeepsAll_WhenRetentionZero(t *testing.T) {
	svc, store, _ := setup()
	ctx := context.Background()

	m, _ := svc.CreateBackup(ctx)
	store.SetCreatedAt(m.ID, time.Now().UTC().AddDate(0, 0, -100))

	if err := svc.CleanupOldBackups(ctx, 0); err != nil {
		t.Fatalf("CleanupOldBackups(0): %v", err)
	}
	list, _ := svc.ListBackups(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1 backup (retention=0 keeps all), got %d", len(list))
	}
}

// --- Restore tests ---

func TestRestoreBackup_TarGz(t *testing.T) {
	svc, _, dumper := setup()
	dump := []byte("-- PostgreSQL database dump\nCOPY users FROM stdin;\n")
	m := makeManifest(dump)
	archive := buildTarGz(t, m, dump, nil)

	if err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive)); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if !bytes.Equal(dumper.Restored, dump) {
		t.Fatalf("expected restored data to match dump")
	}
}

func TestRestoreBackup_TarGz_SHA256Mismatch(t *testing.T) {
	svc, _, _ := setup()
	dump := []byte("-- dump data\n")
	m := makeManifest(dump)
	m.DumpSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	archive := buildTarGz(t, m, dump, nil)

	err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive))
	if err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("sha256 mismatch must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_DumperError(t *testing.T) {
	svc, _, dumper := setup()
	dumper.Err = errors.New("restore failed")

	dump := []byte("-- dump\n")
	m := makeManifest(dump)
	archive := buildTarGz(t, m, dump, nil)

	err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive))
	if err == nil {
		t.Fatal("expected error when dumper fails")
	}
	// Internal postgres/dumper failure must NOT be classified as a client input error.
	if errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("dumper error must NOT satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_EmptySHA256_Rejected(t *testing.T) {
	// An archive whose manifest has no DumpSHA256 must be rejected (fail-closed).
	// Legitimate backups always carry a checksum; an empty field indicates a
	// pre-feature or tampered archive.
	svc, _, _ := setup()
	dump := []byte("-- dump with no sha\n")
	m := makeManifest(dump)
	m.DumpSHA256 = ""
	archive := buildTarGz(t, m, dump, nil)

	err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive))
	if err == nil {
		t.Fatal("expected error when manifest DumpSHA256 is empty")
	}
	if !strings.Contains(err.Error(), "missing dump checksum") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("missing checksum must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_ValidSHA256_Succeeds(t *testing.T) {
	// Restore with a correct matching checksum must succeed and deliver the dump.
	svc, _, dumper := setup()
	dump := []byte("-- valid dump with matching sha\n")
	m := makeManifest(dump) // sets DumpSHA256 = sha256(dump)
	archive := buildTarGz(t, m, dump, nil)

	if err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive)); err != nil {
		t.Fatalf("RestoreBackup with valid SHA256 should succeed: %v", err)
	}
	if !bytes.Equal(dumper.Restored, dump) {
		t.Fatal("restored data should match the original dump")
	}
}

func TestRestoreBackup_InvalidGzip(t *testing.T) {
	svc, _, _ := setup()
	err := svc.RestoreBackup(context.Background(), bytes.NewReader([]byte("not gzip data")))
	if err == nil {
		t.Fatal("expected error for invalid gzip")
	}
	if !strings.Contains(err.Error(), "restore:") {
		t.Fatalf("expected wrapped restore error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("invalid gzip must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_MissingManifest(t *testing.T) {
	svc, _, _ := setup()
	// build tar.gz with only dump.sql, no manifest.json
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTar(t, tw, "dump.sql", []byte("-- dump\n"))
	tw.Close()
	gw.Close()

	err := svc.RestoreBackup(context.Background(), &buf)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	if !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("expected manifest missing error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("missing manifest must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_OversizedDumpRejected(t *testing.T) {
	svc, _, _ := setup()

	// Lower the gzip-bomb guard so we can trip it without a multi-GiB fixture.
	orig := maxRestoreDumpBytes
	maxRestoreDumpBytes = 16
	defer func() { maxRestoreDumpBytes = orig }()

	manifest := &backupmodel.BackupManifest{ID: "bk-x", Version: "1.0.0"}
	manifestBytes, _ := json.Marshal(manifest)
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTar(t, tw, "manifest.json", manifestBytes)
	writeTar(t, tw, "dump.sql", bytes.Repeat([]byte("A"), 64)) // > 16 byte limit
	tw.Close()
	gw.Close()

	err := svc.RestoreBackup(context.Background(), &buf)
	if err == nil {
		t.Fatal("expected error for oversized dump.sql")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected oversized dump error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("oversized dump.sql must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_MissingDump(t *testing.T) {
	svc, _, _ := setup()
	manifest := &backupmodel.BackupManifest{ID: "bk-x", Version: "1.0.0"}
	manifestBytes, _ := json.Marshal(manifest)
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTar(t, tw, "manifest.json", manifestBytes)
	tw.Close()
	gw.Close()

	err := svc.RestoreBackup(context.Background(), &buf)
	if err == nil {
		t.Fatal("expected error for missing dump.sql")
	}
	if !strings.Contains(err.Error(), "dump.sql") {
		t.Fatalf("expected dump missing error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("missing dump.sql must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_ReadError(t *testing.T) {
	// An upload stream that fails to read is indistinguishable from a corrupt
	// archive at the gzip layer - it is classified as ErrInvalidBackup (client
	// error), not an internal server fault.
	svc, _, _ := setup()
	err := svc.RestoreBackup(context.Background(), &errReader{err: errors.New("read broken")})
	if err == nil {
		t.Fatal("expected error from reader")
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("upload read failure must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_CorrectSHA256_Validates(t *testing.T) {
	svc, _, dumper := setup()
	dump := []byte("SELECT 1;\nINSERT INTO users VALUES(1);\n")
	m := makeManifest(dump)
	archive := buildTarGz(t, m, dump, nil)

	if err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive)); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if !bytes.Equal(dumper.Restored, dump) {
		t.Fatal("dump content mismatch after restore")
	}
}

func TestRestoreBackup_InvalidManifestJSON(t *testing.T) {
	svc, _, _ := setup()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTar(t, tw, "manifest.json", []byte("{invalid json"))
	writeTar(t, tw, "dump.sql", []byte("-- dump\n"))
	tw.Close()
	gw.Close()

	err := svc.RestoreBackup(context.Background(), &buf)
	if err == nil {
		t.Fatal("expected error for invalid manifest JSON")
	}
	if !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected parse manifest error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("invalid manifest JSON must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

// --- Schema version guard ---

// buildTarGzWithSchema is like buildTarGz but sets SchemaVersion on the manifest.
func buildTarGzWithSchema(t *testing.T, schemaVersion string, dump []byte) []byte {
	t.Helper()
	hash := sha256.Sum256(dump)
	m := &backupmodel.BackupManifest{
		ID:            "bk-schema-test",
		CreatedAt:     time.Now().UTC(),
		Version:       "1.0.0",
		SchemaVersion: schemaVersion,
		Tables:        BackupTables,
		DumpSHA256:    hex.EncodeToString(hash[:]),
	}
	return buildTarGz(t, m, dump, nil)
}

func TestRestoreBackup_SchemaVersionMismatch_Rejected(t *testing.T) {
	// The fake returns SchemaVersion "42" as current; the backup carries "41".
	svc, _, dumper := setup()
	dumper.SchemaVersion = "42"
	dump := []byte("-- PostgreSQL database dump\nSET statement_timeout = 0;\n")
	archive := buildTarGzWithSchema(t, "41", dump)

	err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive))
	if err == nil {
		t.Fatal("expected error when schema versions mismatch")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected schema version error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"41"`) || !strings.Contains(err.Error(), `"42"`) {
		t.Fatalf("error should name both versions, got: %v", err)
	}
	// Schema mismatch is NOT a client input error - it is a policy rejection.
	if errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("schema mismatch must NOT satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestRestoreBackup_SchemaVersionMatch_Succeeds(t *testing.T) {
	svc, _, dumper := setup()
	dumper.SchemaVersion = "42"
	dump := []byte("-- PostgreSQL database dump\nSET statement_timeout = 0;\n")
	archive := buildTarGzWithSchema(t, "42", dump)

	if err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive)); err != nil {
		t.Fatalf("expected success when schema versions match, got: %v", err)
	}
}

func TestRestoreBackup_SchemaVersionEmpty_Skipped(t *testing.T) {
	// Backup with empty SchemaVersion (pre-feature) must succeed with a warning,
	// not a rejection - preserving restorability of existing backups.
	svc, _, dumper := setup()
	dumper.SchemaVersion = "42"
	dump := []byte("-- PostgreSQL database dump\nSET statement_timeout = 0;\n")
	// buildTarGzWithSchema with "" sets an empty SchemaVersion.
	archive := buildTarGzWithSchema(t, "", dump)

	if err := svc.RestoreBackup(context.Background(), bytes.NewReader(archive)); err != nil {
		t.Fatalf("legacy backup (empty SchemaVersion) must be accepted, got: %v", err)
	}
}

// --- Create edge cases ---

func TestCreateBackup_DumpError(t *testing.T) {
	svc, _, dumper := setup()
	dumper.Err = errors.New("db down")

	_, err := svc.CreateBackup(context.Background())
	if err == nil {
		t.Fatal("expected error when dump fails")
	}
	if !strings.Contains(err.Error(), "db down") {
		t.Fatalf("expected dump error, got %v", err)
	}
}

// trackedRC reports whether Close was called - proves CreateBackup drains/closes
// the dump pipe so the pg_dump writer goroutine can't wedge on an error path.
type trackedRC struct {
	io.Reader
	closed *bool
}

func (t *trackedRC) Close() error { *t.closed = true; return nil }

type closeTrackingDumper struct {
	data   string
	closed *bool
}

func (d closeTrackingDumper) DumpAll(_ context.Context, _ bool) (io.ReadCloser, error) {
	return &trackedRC{Reader: strings.NewReader(d.data), closed: d.closed}, nil
}
func (d closeTrackingDumper) RestoreTables(_ context.Context, _ io.Reader) error { return nil }
func (d closeTrackingDumper) CurrentSchemaVersion(_ context.Context) (string, error) {
	return "", nil
}

func TestCreateBackup_ClosesDumpReader(t *testing.T) {
	closed := false
	store := fakes.NewFakeBackupStore()
	svc := NewService(store, closeTrackingDumper{data: "-- dump\n", closed: &closed}, "1.0.0")
	if _, err := svc.CreateBackup(context.Background()); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if !closed {
		t.Error("CreateBackup must Close the dump ReadCloser (else the pg_dump pipe goroutine + child leak on error paths)")
	}
}

func TestCreateBackup_StoreError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "1.0.0")
	store.Err = errors.New("disk full")

	_, err := svc.CreateBackup(context.Background())
	if err == nil {
		t.Fatal("expected error when store fails")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestCreateBackup_MetadataFields(t *testing.T) {
	svc, _, _ := setup()
	before := time.Now().UTC().Add(-time.Second)

	m, err := svc.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if !strings.HasPrefix(m.ID, "bk-") {
		t.Errorf("expected ID prefix bk-, got %s", m.ID)
	}
	if m.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before test start %v", m.CreatedAt, before)
	}
	if m.DumpSHA256 == "" {
		t.Error("expected DumpSHA256 to be set")
	}
	if len(m.Tables) != len(BackupTables) {
		t.Errorf("expected %d tables, got %d", len(BackupTables), len(m.Tables))
	}
}

// --- List/Delete edge cases ---

func TestListBackups_Empty(t *testing.T) {
	svc, _, _ := setup()
	list, err := svc.ListBackups(context.Background())
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 backups, got %d", len(list))
	}
}

func TestListBackups_StoreError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "1.0.0")
	store.Err = errors.New("list failed")

	_, err := svc.ListBackups(context.Background())
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestDeleteBackup_StoreError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "1.0.0")
	store.Err = errors.New("delete broken")

	err := svc.DeleteBackup(context.Background(), "any-id")
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestGetBackup_DataMatchesDump(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	dumper.DumpData = []byte("exact dump content")
	svc := NewService(store, dumper, "1.0.0")

	m, _ := svc.CreateBackup(context.Background())
	_, rc, err := svc.GetBackup(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if !bytes.Equal(data, dumper.DumpData) {
		t.Fatalf("expected stored data %q, got %q", dumper.DumpData, data)
	}
}

// --- Scheduler ---

func TestStartScheduler_ZeroInterval(t *testing.T) {
	svc, _, _ := setup()
	svc.StartScheduler(t.Context(), 0, 30)

	list, _ := svc.ListBackups(t.Context())
	if len(list) != 0 {
		t.Fatal("scheduler with interval=0 should not create backups")
	}
}

func TestStartScheduler_NegativeInterval(t *testing.T) {
	svc, _, _ := setup()
	svc.StartScheduler(t.Context(), -1, 30)

	list, _ := svc.ListBackups(t.Context())
	if len(list) != 0 {
		t.Fatal("scheduler with negative interval should not create backups")
	}
}

// TestRestoreBackup_ManifestTooLarge asserts that a manifest.json entry larger
// than 16 MiB is rejected as ErrInvalidBackup (gzip-bomb guard).
func TestRestoreBackup_ManifestTooLarge(t *testing.T) {
	svc, _, _ := setup()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	// 17 MiB of junk - well beyond the 16 MiB cap.
	oversized := bytes.Repeat([]byte("a"), 17<<20)
	writeTar(t, tw, "manifest.json", oversized)
	writeTar(t, tw, "dump.sql", []byte("-- dump\n"))
	tw.Close()
	gw.Close()

	err := svc.RestoreBackup(context.Background(), &buf)
	if err == nil {
		t.Fatal("expected error for oversized manifest.json")
	}
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("oversized manifest must satisfy errors.Is(err, ErrInvalidBackup), got: %v", err)
	}
}

func TestWithEventBus_PublishesBackupFailedOnScheduledError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	dumper.Err = errors.New("db unreachable")
	bus := &fakeEventCollector{}
	svc := NewService(store, dumper, "1.0.0", WithEventBus(bus))

	// Exercise the PRODUCTION scheduled-cycle path: a failing dump must make the
	// service ITSELF emit backup.failed (for alerting). Previously this test
	// hand-published the event, so the real publish branch was never covered - a
	// regression that stopped emitting backup.failed would have passed green.
	svc.runScheduledBackupCycle(context.Background(), 0)

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	bf, ok := bus.events[0].(backupmodel.BackupFailed)
	if !ok {
		t.Fatalf("expected BackupFailed event, got %T", bus.events[0])
	}
	if !strings.Contains(bf.Error, "db unreachable") {
		t.Fatalf("expected error containing 'db unreachable', got %q", bf.Error)
	}
	if bf.EventName() != "backup.failed" {
		t.Fatalf("expected event name 'backup.failed', got %q", bf.EventName())
	}
}

// A manual backup is announced too: backup.created with its id and size on
// success, backup.failed on error - not only the scheduled ones.
func TestCreateBackup_AnnouncesManualBackups(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	bus := &fakeEventCollector{}
	svc := NewService(store, fakes.NewFakeDatabaseDumper(), "1.0.0", WithEventBus(bus))

	m, err := svc.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	bc, ok := bus.events[0].(backupmodel.BackupCreated)
	if !ok || bc.EventName() != "backup.created" {
		t.Fatalf("expected backup.created, got %T", bus.events[0])
	}
	if bc.BackupID != m.ID || bc.SizeBytes != m.Size || bc.SizeBytes == 0 {
		t.Fatalf("event %+v does not describe backup %s (%d bytes)", bc, m.ID, m.Size)
	}

	failing := fakes.NewFakeDatabaseDumper()
	failing.Err = errors.New("db unreachable")
	bus = &fakeEventCollector{}
	svc = NewService(store, failing, "1.0.0", WithEventBus(bus))
	if _, err := svc.CreateBackup(context.Background()); err == nil {
		t.Fatal("expected the dump error")
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	if _, ok := bus.events[0].(backupmodel.BackupFailed); !ok {
		t.Fatalf("expected backup.failed for a manual backup, got %T", bus.events[0])
	}
}

// --- Cleanup edge cases ---

func TestCleanupOldBackups_NegativeRetention(t *testing.T) {
	svc, store, _ := setup()
	ctx := context.Background()

	m, _ := svc.CreateBackup(ctx)
	store.SetCreatedAt(m.ID, time.Now().UTC().AddDate(0, 0, -999))

	if err := svc.CleanupOldBackups(ctx, -1); err != nil {
		t.Fatalf("CleanupOldBackups(-1): %v", err)
	}
	list, _ := svc.ListBackups(ctx)
	if len(list) != 1 {
		t.Fatalf("expected negative retention to keep all, got %d", len(list))
	}
}

func TestCleanupOldBackups_RetentionBoundary(t *testing.T) {
	t.Run("retentionDays=0 keeps all", func(t *testing.T) {
		svc, store, _ := setup()
		ctx := context.Background()
		m, _ := svc.CreateBackup(ctx)
		store.SetCreatedAt(m.ID, time.Now().UTC().AddDate(0, 0, -999))
		if err := svc.CleanupOldBackups(ctx, 0); err != nil {
			t.Fatalf("CleanupOldBackups(0): %v", err)
		}
		list, _ := svc.ListBackups(ctx)
		if len(list) != 1 {
			t.Fatalf("retentionDays=0 should keep all backups, got %d", len(list))
		}
	})
	t.Run("retentionDays=1 deletes old", func(t *testing.T) {
		svc, store, _ := setup()
		ctx := context.Background()
		old, _ := svc.CreateBackup(ctx)
		store.SetCreatedAt(old.ID, time.Now().UTC().AddDate(0, 0, -2))
		if err := svc.CleanupOldBackups(ctx, 1); err != nil {
			t.Fatalf("CleanupOldBackups(1): %v", err)
		}
		list, _ := svc.ListBackups(ctx)
		if len(list) != 0 {
			t.Fatalf("retentionDays=1 should delete 2-day-old backup, got %d remaining", len(list))
		}
	})
}

func TestCleanupOldBackups_ExactCutoffBoundary(t *testing.T) {
	t.Run("backup well after cutoff is kept", func(t *testing.T) {
		svc, store, _ := setup()
		ctx := context.Background()
		m, _ := svc.CreateBackup(ctx)
		store.SetCreatedAt(m.ID, time.Now().UTC().AddDate(0, 0, -29))

		if err := svc.CleanupOldBackups(ctx, 30); err != nil {
			t.Fatalf("CleanupOldBackups: %v", err)
		}
		list, _ := svc.ListBackups(ctx)
		if len(list) != 1 {
			t.Fatalf("backup newer than cutoff should be kept, got %d remaining", len(list))
		}
	})
	t.Run("backup just before cutoff is deleted", func(t *testing.T) {
		svc, store, _ := setup()
		ctx := context.Background()
		m, _ := svc.CreateBackup(ctx)
		store.SetCreatedAt(m.ID, time.Now().UTC().AddDate(0, 0, -30).Add(-time.Second))

		if err := svc.CleanupOldBackups(ctx, 30); err != nil {
			t.Fatalf("CleanupOldBackups: %v", err)
		}
		list, _ := svc.ListBackups(ctx)
		if len(list) != 0 {
			t.Fatalf("backup older than cutoff should be deleted, got %d remaining", len(list))
		}
	})
	t.Run("retentionDays=-1 keeps everything", func(t *testing.T) {
		svc, store, _ := setup()
		ctx := context.Background()
		m, _ := svc.CreateBackup(ctx)
		store.SetCreatedAt(m.ID, time.Now().UTC().AddDate(-10, 0, 0))

		if err := svc.CleanupOldBackups(ctx, -1); err != nil {
			t.Fatalf("CleanupOldBackups(-1): %v", err)
		}
		list, _ := svc.ListBackups(ctx)
		if len(list) != 1 {
			t.Fatalf("negative retention should keep all, got %d", len(list))
		}
	})
}

func TestCleanupOldBackups_ListError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "1.0.0")
	store.Err = errors.New("list broken")

	err := svc.CleanupOldBackups(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error from store.List")
	}
}

func TestCreateBackup_ForwardsIncludeTelemetry(t *testing.T) {
	t.Run("default excludes telemetry", func(t *testing.T) {
		store := fakes.NewFakeBackupStore()
		dumper := fakes.NewFakeDatabaseDumper()
		dumper.DumpData = []byte("-- PostgreSQL database dump\n")
		svc := NewService(store, dumper, "1.0.0")
		if _, err := svc.CreateBackup(context.Background()); err != nil {
			t.Fatalf("CreateBackup: %v", err)
		}
		if dumper.LastIncludeTelemetry {
			t.Error("default backup must exclude telemetry")
		}
	})
	t.Run("opt-in includes telemetry", func(t *testing.T) {
		store := fakes.NewFakeBackupStore()
		dumper := fakes.NewFakeDatabaseDumper()
		dumper.DumpData = []byte("-- PostgreSQL database dump\n")
		svc := NewService(store, dumper, "1.0.0", WithIncludeTelemetry(func() bool { return true }))
		if _, err := svc.CreateBackup(context.Background()); err != nil {
			t.Fatalf("CreateBackup: %v", err)
		}
		if !dumper.LastIncludeTelemetry {
			t.Error("WithIncludeTelemetry(true) must forward to the dumper")
		}
	})
}
