package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// adminCommand is the single description of one `evey admin` subcommand: the
// dispatch, the usage list, `--help`, the manual and the completion drift test
// all read it, so a subcommand cannot be added to one and forgotten in another.
type adminCommand struct {
	name  string
	args  string // synopsis after the name
	short string // one line
	long  string // paragraphs, blank-line separated
	group string
	// noFlags marks a subcommand that parses its own arguments (no FlagSet):
	// runAdmin answers its --help from this table instead.
	noFlags bool
	run     func(args []string) error
}

// The handlers are bound here rather than in the table: they build their
// FlagSet through adminFlags, which reads the table, and Go refuses that cycle.
var adminRunners = map[string]func([]string) error{
	"reset-password":     adminResetPassword,
	"rotate-jwt-key":     adminRotateJWTKey,
	"backup":             adminBackup,
	"restore-backup":     adminRestoreBackup,
	"reindex-search":     adminReindexSearch,
	"migrate":            adminMigrate,
	"reclassify":         adminReclassify,
	"reindex-no-cue":     adminReindexNoCue,
	"validate-streaming": adminValidateStreaming,
	"artwork-gc":         adminArtworkGC,
	"load-test":          adminLoadTest,
	"doctor":             adminDoctor,
	"rollback":           adminRollback,
	"ladder-bench":       adminLadderBench,
	"ladder-probe":       adminLadderProbe,
	"route-bench":        adminRouteBench,
	"list-users":         adminListUsers,
	"list-libraries":     adminListLibraries,
	"demo-seed":          adminDemoSeed,
}

func init() {
	for i := range adminCommands {
		run, ok := adminRunners[adminCommands[i].name]
		if !ok {
			panic("admin subcommand without a handler: " + adminCommands[i].name)
		}
		adminCommands[i].run = run
	}
}

const (
	groupRecovery    = "Recovery"
	groupSchema      = "Schema"
	groupEncoding    = "Per-title encoding"
	groupMaintenance = "Media maintenance"
	groupCapacity    = "Capacity (need a LIVE server - HTTP client of the local gateway)"
	groupDiagnostics = "Diagnostics"
	groupCompletion  = "Completion sources"
)

var adminGroups = []string{groupRecovery, groupSchema, groupEncoding, groupMaintenance, groupCapacity, groupDiagnostics, groupCompletion}

var adminCommands = []adminCommand{
	{
		name: "reset-password", args: "--user <email>", group: groupRecovery,
		short: "Generate a new password for the user, print it once.",
		long: "Generates a fresh random password for an existing user, stores its hash and prints the plaintext once to stdout. " +
			"Every session of that user is ended at the same time.",
	},
	{
		name: "rotate-jwt-key", group: groupRecovery,
		short: "Regenerate the JWT signing key. Invalidates ALL active sessions.",
		long: "Moves the RSA signing key aside (kept as <key>.rotated-<timestamp>); the next server boot generates a new one. " +
			"Every session is ended in the database at once (refresh tokens deleted, access tokens revoked), so every user must sign in again.",
	},
	{
		name: "backup", args: "[--include-telemetry]", group: groupRecovery,
		short: "Create a database backup (same artifact as the panel and the scheduler).",
		long: "Writes a backup to the configured backup directory. Telemetry tables are excluded by default to keep it lean; " +
			"pass --include-telemetry to dump them too, or leave it off to follow the saved panel setting.",
	},
	{
		name: "restore-backup", args: "<path-to-dump.sql.gz>", group: groupRecovery, noFlags: true,
		short: "Restore the database from a pg_dump.gz file.",
		long: "Feeds a .sql.gz dump (as written by the pre-migration backup) through gunzip and psql into the configured database, " +
			"in a single transaction. Existing rows are OVERWRITTEN; the command waits 5 seconds for a Ctrl-C before it starts. " +
			"psql and gunzip must be on PATH.",
	},
	{
		name: "reindex-search", args: "[--library <id>] [--purge]", group: groupRecovery,
		short: "Rebuild the Meilisearch index from the database (no metadata fetch).",
		long: "Reads the media items straight from Postgres and upserts them into Meilisearch. --purge clears the index first, " +
			"which drops the orphan documents an upsert cannot (brief empty-search window). If Meilisearch is down or not " +
			"configured, search is served by the Postgres fallback, which needs no index: the command reports that and does nothing.",
	},
	{
		name: "migrate", args: "<up|down|version> [--no-backup]", group: groupSchema, noFlags: true,
		short: "Apply pending migrations, roll back one step, or print the schema version.",
		long: "up applies every pending migration; down rolls back exactly one step; version prints the current schema version " +
			"and exits 1 when a previous migration failed halfway (dirty state).\n\n" +
			"A pre-migration pg_dump is taken for up and down when EVEY_BACKUP_DIR is set; --no-backup skips it " +
			"(hosts without pg_dump, such as a WSL dev shell).",
	},
	{
		name: "reclassify", args: "<media-id> | --all", group: groupEncoding, noFlags: true,
		short: "Force-recompute the complexity class of one media item, or re-queue every classified item.",
		long: "Wipes the per-title complexity row so the background worker classifies the item again on its next tick. " +
			"--all wipes every row; the worker then re-classifies the whole library at its background pace.",
	},
	{
		name: "reindex-no-cue", args: "--root <dir> [--apply] [--limit N] [--mode auto|inplace|rewrite] [--deep-verify] [--resync] [--keep-backup=false]",
		group: groupMaintenance,
		short: "Rebuild the seek index (Cues) of MKV/WebM files that lack one, fixing slow resume.",
		long: "Dry-run by default: pass --apply to rewrite. Each rewrite is validated and cross-checked and keeps a .mkvgobak " +
			"backup; --limit N for a cautious first run; --keep-backup=false drops the backups and enforces --deep-verify instead. " +
			"--mode auto takes the surgical in-place patch when the file layout allows it and rewrites otherwise; inplace patches " +
			"only (no backup) and refuses defeated layouts; rewrite always rebuilds and swaps.\n\n" +
			"--resync repairs files with a CORRUPTED region the strict pass refuses: it skips the damage, keeps the rest and " +
			"reports the lost bytes. LONG: rewrites each file in full (hours on a large library); shows a progress bar while applying.",
	},
	{
		name: "validate-streaming", args: "--root <dir> [--errors-only]", group: groupMaintenance,
		short: "Report each MKV/WebM file's streaming readiness (Cues index, keyframe cues, DefaultDuration).",
		long: "Read-only. Reports what the on-demand HLS copy rung and seeking rely on: a missing Cues index, cue points on a " +
			"non-video track, cue times matching no keyframe, video tracks without a DefaultDuration. A file with errors falls " +
			"back to the ffmpeg pipeline; reindex-no-cue is the fix for a missing index. --errors-only skips the warnings.",
	},
	{
		name: "artwork-gc", args: "[--root <dir>] [--apply]", group: groupMaintenance,
		short: "Delete artwork files on disk that no DB row references.",
		long: "Reclaims the orphans left by the old UUID-named artwork scheme. Dry-run by default: prints what it would delete; " +
			"--apply deletes. --root defaults to /data/artworks.",
	},
	{
		name: "load-test", args: "[minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]",
		group: groupCapacity,
		short: "Ramp simulated viewers until the box hits its ceiling; report the sustained count and latencies.",
		long: "Needs a live server: it is an HTTP client of the local gateway and mints itself a short-lived admin token from " +
			"the JWT key. Scenarios: transcode, natural, mkvgo, light (80/20), balanced (50/50, default), heavy (20/80), " +
			"mkvgo+N (N pinned transcodes, then pure direct-play). Modes: ramp (default), flash (everyone at once), seek " +
			"(viewers jump around), endurance (hold the tier). The preset sizes the ramp: minimal (4 viewers), standard " +
			"(server default, 12), max (32); an explicit --max-viewers wins. --cold-cache keeps evicting the library from " +
			"the page cache so the run measures the storage, not the RAM.\n\n" +
			"Ctrl-C offers to cancel. admission_full is the ceiling verdict (exit 0), not an error. Short sanity run: " +
			"load-test --scenario balanced --max-viewers 4.",
	},
	{
		name: "doctor", args: "[--repair]", group: groupDiagnostics,
		short: "Health-check DB, schema, jwt.key, Meilisearch, media mounts, cache dirs, ffmpeg/mkvgo, remote access and disk.",
		long: "One report, one line per check (ok / WARN / FAIL) with a remediation hint. Checks the search index for drift " +
			"against the database, whether the server identity can write each library root, and the remote-access gate " +
			"(a published server that refuses every remote client is a FAIL). --repair applies only the safe fixes: " +
			"create and chown the missing writable dirs. Exits 1 when a check fails.",
	},
	{
		name: "rollback", group: groupRecovery, noFlags: true,
		short: "Read-only advisor: running version, schema, newest pre-migration backup and the exact HOST commands to roll back.",
		long: "The container has no Docker access, by design, so it cannot swap its own image: this prints what to run on the host, " +
			"choosing the previous image tag, and the restore step to take only if the newer version applied migrations.",
	},
	{
		name: "ladder-bench", args: "[--scenes 3] [--window 8] [--heights 1080,720] [--crf 18,23,28] [--out rows.csv] [--efficiency --mode full|fast|incremental] <file>...",
		group: groupDiagnostics,
		short: "Measure the ladder engine's complexity features on a few scenes of each title and score encode cost.",
		long: "Encodes a few scenes of each title at fixed CRFs and scores bitrate and XPSNR; prints one CSV row per scene " +
			"and the rank correlation of every feature with the encode cost. --efficiency switches to the codec/encoder " +
			"efficiency curves per production encoder. Runs ffmpeg for minutes; never on a box that is serving viewers.",
	},
	{
		name: "ladder-probe", args: "[--window 8] [--cache <dir>] [--out <dir>] <file>...", group: groupDiagnostics,
		short: "Run the per-title probe on the given files and report probe time, early-stop rate and points.",
		long: "Reads and encodes only; nothing touches the library. Reports per title the probe wall time, whether the " +
			"cost model was confirmed and how many points it took; --out writes each title's probe JSON.",
	},
	{
		name: "route-bench", args: "[--backend nvenc|vaapi|qsv|none] [--heights 1080,2160] [--fps 24,30,50,60] [--out rows.csv] [--json run.json] [<file>...]",
		group: groupDiagnostics,
		short: "Measure what a session of each transcode route costs on this host against the calibration and the cost model.",
		long: "Routes: GPU zero-copy, bridge, tone-map, passthrough, decoupled, CPU. Measures fps, CPU cores and GPU engines on " +
			"synthetic clips per frame rate plus any real files given. Runs the production calibration first unless " +
			"--capability points at a persisted one. Minutes of full load; never while serving.",
	},
	{
		name: "demo-seed", args: "[--apply] [--media]", group: groupRecovery,
		short: "Run the demo bootstrap by hand: the demo administrator, the visitor accounts, the four libraries, optionally the media.",
		long: "Refuses unless EVEY_DEMO=1. The server does the same at every boot with EVEY_DEMO=1; this seeds an instance ahead " +
			"of its first boot or shows what the bootstrap would do. Creates the demo/demo administrator (through the setup path " +
			"when no administrator exists), the accounts a visitor can \"connect as\" (a household with a kids profile behind a PIN, " +
			"a viewer with a device cap and a bitrate ceiling, a guest confined to the films, a disabled account), the four " +
			"libraries under EVEY_DEMO_MEDIA_DIR, and with --media the freely licensed films, episodes, tracks and photos of the " +
			"embedded manifest (gigabytes). Idempotent: what exists is left alone. Dry-run by default; --apply writes.",
	},
	{
		name: "list-users", group: groupCompletion, noFlags: true,
		short: "Print user emails, one per line (drives reset-password --user Tab-completion).",
	},
	{
		name: "list-libraries", group: groupCompletion, noFlags: true,
		short: "Print every library root path, one per line (drives --root Tab-completion).",
	},
}

func findAdminCommand(name string) *adminCommand {
	for i := range adminCommands {
		if adminCommands[i].name == name {
			return &adminCommands[i]
		}
	}
	return nil
}

func isHelpArg(a string) bool { return a == "-h" || a == "--help" || a == "-help" || a == "help" }

// adminFlags is the FlagSet every subcommand parses with: -h prints the
// table's description above Go's flag defaults, so --help reads the same
// everywhere.
func adminFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		if cmd := findAdminCommand(name); cmd != nil {
			printAdminCommandHelp(fs.Output(), cmd)
		}
		if hasFlags(fs) {
			fmt.Fprintln(fs.Output(), "Flags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

func hasFlags(fs *flag.FlagSet) bool {
	n := 0
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n > 0
}

func printAdminCommandHelp(w io.Writer, cmd *adminCommand) {
	fmt.Fprintf(w, "usage: evey admin %s\n\n", strings.TrimSpace(cmd.name+" "+cmd.args))
	fmt.Fprintln(w, wrapText(cmd.short, 78, ""))
	if cmd.long != "" {
		for p := range strings.SplitSeq(cmd.long, "\n\n") {
			fmt.Fprintln(w)
			fmt.Fprintln(w, wrapText(p, 78, ""))
		}
	}
	fmt.Fprintln(w)
}

func printAdminUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: evey admin <subcommand> [flags]")
	fmt.Fprintln(w, "       evey admin <subcommand> --help")
	for _, g := range adminGroups {
		fmt.Fprintf(w, "\n%s:\n", g)
		for _, c := range adminCommands {
			if c.group != g {
				continue
			}
			fmt.Fprintf(w, "  %s\n", strings.TrimSpace(c.name+" "+c.args))
			fmt.Fprintln(w, wrapText(c.short, 72, "      "))
		}
	}
	fmt.Fprintln(w, `
Configuration is read from EVEY_* env vars (same as the server).
Full reference: evey man --text | more, or docs/runbooks/cli.md.`)
}

func adminUsage() error {
	printAdminUsage(os.Stderr)
	return fmt.Errorf("admin: subcommand required")
}

// wrapText fills text into lines of at most width columns, each prefixed.
func wrapText(text string, width int, prefix string) string {
	var b strings.Builder
	line := prefix
	for w := range strings.FieldsSeq(text) {
		if len(line) > len(prefix) && len(line)+1+len(w) > width {
			b.WriteString(line)
			b.WriteByte('\n')
			line = prefix
		}
		if len(line) > len(prefix) {
			line += " "
		}
		line += w
	}
	b.WriteString(line)
	return b.String()
}
