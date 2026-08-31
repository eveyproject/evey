// Package maintenance holds admin-triggered media maintenance use cases that
// operate on the on-disk library rather than the database. The seek-index tools
// detect and repair MKV/WebM files muxed without a Cues index (the cause of slow
// seek/resume) using the fail-safe mkv.Reindexer port.
//
// Every entry point is asynchronous: scanning walks thousands of files on a
// (possibly network-mounted) library and repairing rewrites whole multi-GB
// files, so a synchronous RPC would time out behind any reverse proxy (the
// prod 504). Callers start a job and poll Status(); completed scans are cached
// in memory and served instantly.
package maintenance

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// ErrJobRunning is returned by the Start* entry points when a seek-index job
// is already in flight. Handlers map it to a user-actionable "try again later"
// instead of a 500. Jobs are serialized globally to bound disk I/O on weak
// hosts (NAS), matching the sequential batch design.
var ErrJobRunning = errors.New("a seek-index job is already running")

// Job kinds and phases surfaced by Status().
const (
	JobScan     = "scan"
	JobReindex  = "reindex"
	JobRollback = "rollback"
	JobPurge    = "purge" // delete every .mkvgobak backup in scope
	// JobRemuxAudio rewrites files whose audio content starts late (repack
	// mux defect) so the file itself is fixed, not just the playback.
	JobRemuxAudio = "remux_audio"
	// JobFixContainer renames .mkv files whose bytes are really MP4/MOV to
	// .mp4 and repoints the catalog: there is no Matroska index to build in
	// them, the extension is the defect.
	JobFixContainer = "fix_container"

	PhaseScanning   = "scanning"
	PhaseProcessing = "processing"
)

// maxFailures bounds the per-job failure list kept for the status endpoint.
const maxFailures = 50

// MediaRef is the minimal media-file info the seek-index tools need. It is
// resolved from the library by a MediaCatalog adapter.
type MediaRef struct {
	MediaID string
	Title   string
	Path    string
}

// MediaCatalog lists and looks up media files. Satisfied by an adapter over the
// library query service (kept narrow so this package doesn't depend on the
// whole library app service).
type MediaCatalog interface {
	ListMediaFiles(ctx context.Context, libraryID string) ([]MediaRef, error)
	GetMediaFile(ctx context.Context, mediaID string) (MediaRef, error)
}

// FileStateRefresher re-stamps the change-detection fingerprint (size + mtime)
// the library scan keys on for a media file. After a reindex rewrites a
// container in place, the bytes change but the logical content - streams,
// keyframe times, duration - does not; refreshing the stored fingerprint makes
// the next scan treat the file as UNCHANGED instead of re-probing it and
// dropping the keyframe index, which would force a transcode window (the copy
// rung needs keyframes_ms) until the keyframe backfill worker catches up.
// Implemented by the media-item repository; optional (nil disables the refresh,
// falling back to the re-probe path).
type FileStateRefresher interface {
	RefreshFileState(ctx context.Context, path string, size int64, modTime time.Time) error
}

// AudioDelayProber measures how far a file's audio content starts AFTER its
// video (max across audio tracks, seconds) - the repack mux defect that plays
// as a constant A/V desync. Satisfied by the ffmpeg prober; optional (nil
// skips the diagnosis). Kept separate from the Cues probe: it is a different
// defect class, repaired by a remux, not a reindex.
type AudioDelayProber interface {
	ProbeMaxAudioStartDelaySec(ctx context.Context, path string) (float64, error)
}

// AudioDelayFlagSec is the floor above which a probed audio start delay is
// surfaced as a defect. Matches the HLS builders' cancel floor: legitimate
// codec priming sits at ~20-40ms, the defect class at hundreds of ms.
const AudioDelayFlagSec = 0.1

// AudioTailProber measures how far BEFORE the container's declared duration
// the audio content actually ends: the largest gap, in seconds, between the
// declared duration and the last audio packet of any audio track (0 = the
// audio runs to the end). The defect class (prod 2026-07-26, "Scary Movie"):
// a mux whose audio track dies minutes before the video's end - structurally
// healthy, invisible to the index/damage checks, yet the VOD playlists promise
// audio segments that can never exist. Satisfied by the ffmpeg adapter
// (per-track DURATION tags head-only, tail packet read as fallback); optional
// (nil skips the probe). Primitive-typed on purpose - the driven adapter
// implements it without importing this package.
type AudioTailProber interface {
	ProbeAudioTailGapSec(ctx context.Context, path string) (float64, error)
}

// AudioShortfallFlagSec is the floor above which an audio track ending before
// the declared duration is surfaced as a defect. Ordinary muxes leave up to a
// couple of seconds of slack (priming, final-cluster rounding); the defect
// class is measured in minutes (249 s on the file that motivated this).
const AudioShortfallFlagSec = 10.0

// AudioDelayRemuxer repairs the audio start-delay defect AT THE FILE level:
// a remux that shifts the late audio content back onto the video, with the
// same fail-safe contract as the reindexer (rewrite to a temp, re-verify,
// atomic swap, original kept as .mkvgobak; any failure leaves the original
// byte-for-byte intact). skipped=true means no track sits above the defect
// floor anymore (nothing to fix); backupPath is the kept original ("" when
// skipped). Satisfied by the ffmpeg adapter; optional (nil disables the
// remux jobs). Primitive-typed on purpose - the driven adapter implements it
// without importing this package.
type AudioDelayRemuxer interface {
	RemuxAudioDelay(ctx context.Context, path string) (backupPath string, skipped bool, err error)
}

// SeekStatus is the per-file diagnosis surfaced by a scan.
type SeekStatus struct {
	MediaID   string
	Title     string
	Path      string
	Container string
	HasIndex  bool // false → muxed without Cues → slow seek/resume
	HasBackup bool // a .mkvgobak exists → a prior reindex is rollback-able
	SizeBytes int64
	// ModTimeUnix is the file's mtime at diagnosis time. Together with
	// SizeBytes it is the change fingerprint an incremental re-scan compares:
	// unchanged → the whole diagnosis is REUSED without touching the file
	// (in prod, diagnosing 6651 files costs ~1h30 of head seeks and
	// first-cluster reads; between two scans virtually none of them changed).
	// The same trade the library scan's scanindex makes: an in-place edit
	// that preserves size AND mtime keeps a stale diagnosis until either
	// moves - deliberate tooling does not do that, and every evey repair
	// re-stamps the fingerprint itself.
	ModTimeUnix int64
	// AudioDelaySec > AudioDelayFlagSec → the audio track starts that far
	// after the video (mux defect → constant A/V desync on copied audio).
	// Playback with TRANSCODED audio auto-cancels it; the flag tells the
	// operator the file itself carries the defect. 0 when not probed.
	AudioDelaySec float64
	// Truncated → the file is an incomplete download whose damage runs to the
	// end: no tool can recover the missing tail, a resync would only produce a
	// playable prefix. The operator must re-acquire the source. Only a
	// diagnosing scan (WithDiagnoser) can tell this apart from Damaged.
	Truncated bool
	// MissingTailBytes quantifies Truncated: how many declared bytes are
	// absent from the end. The magnitude routes the operator's decision - a
	// 9 KB stub is invisible in playback, 41 MB is a missing final scene.
	// 0 when not truncated or not measurable.
	MissingTailBytes int64
	// DamagedBytes > 0 → the file has mid-file corruption a resync repair can
	// skip past, losing this many bytes (repairable in full otherwise). 0 when
	// clean or not diagnosed.
	DamagedBytes int64
	// WrongContainer → a .mkv whose bytes are really MP4/MOV. There is no
	// seek-index to build (its sample table IS its index); the remedy is to
	// rename/remux, not reindex. Kept out of the missing-index bucket so a
	// "Repair all" never wastes a doomed Matroska reindex on it.
	WrongContainer bool
	// TrailingJunk → surplus bytes lie past the declared Segment end, but the
	// media itself is complete (leftover junk, or a crashed in-place-repair
	// journal). Harmless: any reindex rewrite drops them. Distinct from Damaged
	// so a whole file is never shown as corrupt just for carrying tail junk.
	TrailingJunk bool
	// AudioShortfallSec > AudioShortfallFlagSec → an audio track ends that far
	// BEFORE THE VIDEO does (mux defect: silent credits on that track, and -
	// before the playback-side silence fill - a hard player error at the wall).
	// Measured against the video's real end rather than the container's
	// declared duration, which is itself only a muxer's claim: an over-stated
	// duration made every such file look like an audio defect. NOT repairable:
	// the audio's tail is simply absent from the file; playback fills it with
	// silence, a real fix means re-acquiring the source. 0 when clean or not
	// probed.
	AudioShortfallSec float64
	// IndexSparse → the video cues are keyed right but too far apart to seek
	// into (mkvgo "index-sparse"). It is an index defect like the others, so
	// HasIndex is false; this says WHICH one, because it is the only one a
	// rebuild can fail to improve.
	IndexSparse bool
	// IndexSparseDetail is the diagnoser's own account of the hole ("the video
	// cues leave a 44s hole - ..."), shown instead of a generic explanation.
	IndexSparseDetail string `json:"IndexSparseDetail,omitempty"`
	// PictureMissing → the hole in the seek index is a hole in the VIDEO itself
	// (the diagnoser found no picture there): nothing to index, a rebuild is
	// wasted work, the operator re-acquires the source. Manual verdict at once,
	// without the "rebuild once, then give up" round trip SeekCoarse needs.
	PictureMissing bool `json:"PictureMissing,omitempty"`
	// ReindexAttempted → this file's index was actually rewritten by a repair
	// (not skipped) since it was last modified. Set by the repair itself and
	// carried by the incremental re-scan, which only reuses a status whose
	// (size, mtime) fingerprint still matches - so re-acquiring the source
	// clears it, which is exactly right.
	ReindexAttempted bool
	// AckedAt (unix seconds) > 0 → the operator has SEEN this file's verdict
	// and chose to live with it. Only a file nothing can be done about here
	// (a manual or informational verdict, see Verdict) can be acknowledged:
	// a repairable defect is repaired, not waved through. Carried by the
	// incremental re-scan exactly like ReindexAttempted - as long as the
	// (size, mtime) fingerprint holds - so a re-acquired or rewritten file
	// comes back un-acknowledged and gets judged afresh.
	AckedAt int64 `json:"AckedAt,omitempty"`
	// DiagVersion is the version of the DETECTORS that produced this status.
	// The incremental re-scan reuses a status while the file's (size, mtime)
	// fingerprint holds - which is right for the file, and wrong for the
	// detectors: a probe fixed on the 26th (the audio tail measured against
	// the declared duration instead of the video's end) kept every earlier
	// false positive alive in production for a month, because no unchanged
	// file was ever re-judged. A status from an older detector version is
	// re-diagnosed regardless of the fingerprint.
	DiagVersion int `json:"DiagVersion,omitempty"`
}

// SeekDiagVersion is bumped whenever a detector's verdict can change for an
// unchanged file (a new rule, a fixed reference, a moved threshold). The next
// scan then re-diagnoses the whole library once - hours on a NAS, and the
// price of never showing a stale verdict.
//
//	1: implicit (statuses persisted before the field existed)
//	2: audio tail measured against the video end; index-sparse carries mkvgo's detail
const SeekDiagVersion = 2

// Verdict is what the health page owes the operator for every file: not a
// pile of flags, but the ONE answer "who acts on this, if anyone". Three
// non-healthy answers, in decreasing order of what Evey can do:
//
//   - VerdictRepairable: Evey repairs it (index rebuild, resync of a damaged
//     stretch, audio start-delay remux, container rename). "Repair all" acts
//     on exactly these, and its count is exactly their count.
//   - VerdictManual: only the operator can act - re-acquire an incomplete
//     download, re-encode a source whose keyframes are too sparse to seek.
//   - VerdictInformational: a fact about the file that changes nothing for
//     playback (a shorter audio track the player pads with silence, junk
//     bytes after the Segment end). Nothing to do, for anyone.
//
// Manual and informational verdicts can be acknowledged so the page tends
// towards zero instead of carrying the same rows for years.
type Verdict string

const (
	VerdictHealthy       Verdict = "healthy"
	VerdictRepairable    Verdict = "repairable"
	VerdictManual        Verdict = "manual"
	VerdictInformational Verdict = "informational"
)

// Verdict derives the file's single answer from its flags. Precedence: a
// defect only the operator can fix wins over one Evey could fix (repairing
// the index of an incomplete download is wasted work until the source is
// re-acquired), and any actionable defect wins over a mere fact.
func (st SeekStatus) Verdict() Verdict {
	switch {
	case st.Truncated, st.PictureMissing, st.SeekCoarse():
		return VerdictManual
	case !st.HasIndex, st.DamagedBytes > 0, st.AudioDelaySec > AudioDelayFlagSec, st.WrongContainer:
		return VerdictRepairable
	case st.AudioShortfallSec > AudioShortfallFlagSec, st.TrailingJunk:
		return VerdictInformational
	}
	return VerdictHealthy
}

// Acked reports whether the operator acknowledged this file's verdict.
func (st SeekStatus) Acked() bool { return st.AckedAt > 0 }

// Ackable reports whether the verdict is one the operator may acknowledge:
// only what nobody here can repair. A repairable defect gets repaired.
func (st SeekStatus) Ackable() bool {
	v := st.Verdict()
	return v == VerdictManual || v == VerdictInformational
}

// SeekCoarse reports a file whose index was rebuilt and is STILL too coarse to
// seek into: the encode's own keyframes are further apart than a seek can
// tolerate, and no index can be denser than the keyframes it names. Such a file
// is deliberately kept OUT of the repair queue - offering a repair that cannot
// change anything is the sterile loop this whole path exists to avoid (prod
// 2026-07-26: 138 files "repaired", 134 still flagged). The remedy is a
// re-encode or a better source, neither of which evey performs.
func (st SeekStatus) SeekCoarse() bool { return st.IndexSparse && st.ReindexAttempted }

// ScanResult bundles the per-file statuses with rollup counts.
type ScanResult struct {
	Items          []SeekStatus
	Total          int
	MissingIndex   int
	WithBackup     int
	AudioDelayed   int // count with AudioDelaySec > AudioDelayFlagSec
	Truncated      int // count that are incomplete downloads (re-acquire, not repair)
	Damaged        int // count with mid-file corruption (resync-repairable)
	WrongContainer int // count that are MP4/MOV mislabelled .mkv (rename/remux, not reindex)
	TrailingJunk   int // count carrying surplus bytes past the Segment end (harmless, a rewrite drops them)
	AudioShort     int // count whose audio ends before the declared duration (re-acquire; playback fills silence)
	SeekCoarse     int // count rebuilt and still too coarse to seek into (re-encode; no repair can improve them)

	// The triage the page is built on (see Verdict). Repairable is exactly
	// what "Repair all" will act on. Manual and Informational count only the
	// files still AWAITING the operator's eyes: an acknowledged file moves
	// to Acked, whatever its verdict.
	Repairable    int
	Manual        int
	Informational int
	Acked         int
}

// Failure records one file a batch job could not process.
type Failure struct {
	Path     string
	Error    string
	ReadOnly bool // the failure came from a read-only mount (:ro volume)
}

// JobSummary describes the last finished job.
type JobSummary struct {
	Kind       string
	Scope      string // "" = all libraries, else the library ID
	Done       int
	Failed     int
	Skipped    int
	Total      int
	FinishedAt time.Time
	Err        string // job-level error (e.g. the scan itself failed)
	ReadOnly   bool   // at least one failure came from a read-only mount
	Failures   []Failure
}

// JobStatus is the pollable state of the seek-index job runner.
type JobStatus struct {
	Running     bool
	Kind        string
	Scope       string
	Phase       string
	Done        int
	Failed      int
	Skipped     int
	Total       int
	CurrentPath string
	StartedAt   time.Time
	Last        *JobSummary
}

type cachedScan struct {
	res ScanResult
	at  time.Time
}

// SeekIndexService is the admin-facing seek-index maintenance use case.
type SeekIndexService struct {
	reindexer  mkv.Reindexer
	catalog    MediaCatalog
	fileState  FileStateRefresher // optional; see WithFileStateRefresher
	audioDelay AudioDelayProber   // optional; see WithAudioDelayProber
	audioRemux AudioDelayRemuxer  // optional; see WithAudioRemuxer
	audioTail  AudioTailProber    // optional; see WithAudioTailProber
	diagnoser  mkv.Diagnoser      // optional; see WithDiagnoser
	relocator  MediaRelocator     // optional; see WithMediaRelocator
	logger     *slog.Logger

	stateDir string // "" = no persistence (WithStateDir enables it)

	// activeStreams lets a repair batch stand aside while someone is watching.
	// These jobs rewrite multi-gigabyte files in place on the library disk, so
	// running them against a live viewer is the shape of the 2026-07-19
	// incident, where a background rewrite starved playback of I/O. Optional:
	// nil means never yield (the prior behaviour).
	activeStreams ActiveStreamChecker
	// idlePoll is how often the batch re-checks whether playback has stopped.
	idlePoll time.Duration

	mu          sync.Mutex
	running     bool
	cancel      context.CancelFunc // non-nil while a job runs; see Cancel
	cur         JobStatus          // valid while running (Last excluded)
	curFailures jobFailures
	last        *JobSummary
	scans       map[string]cachedScan // scope key ("*" or libraryID) → last completed scan
}

// ActiveStreamChecker reports whether ANY playback is currently reading the
// disk, whatever machinery serves it. Deliberately not ActiveSessionCount: that
// counts ffmpeg sessions only, and a seek-index rewrite must also hold off for
// a direct-play or mkvgo-served viewer it would otherwise compete with. Same
// shape as the trickplay generator's checker, so one concrete type satisfies
// both.
type ActiveStreamChecker interface {
	ActivePlaybackCount() int
}

// defaultIdlePoll matches the trickplay generator's backoff: long enough that
// waiting costs nothing, short enough that a repair resumes promptly once the
// last viewer stops.
const defaultIdlePoll = 30 * time.Second

func NewSeekIndexService(reindexer mkv.Reindexer, catalog MediaCatalog, logger *slog.Logger) *SeekIndexService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SeekIndexService{
		reindexer: reindexer,
		catalog:   catalog,
		logger:    logger.With("component", "seek_index"),
		scans:     map[string]cachedScan{},
		idlePoll:  defaultIdlePoll,
	}
}

// WithFileStateRefresher wires the post-reindex fingerprint refresh so a
// rewritten file is not re-probed (and its keyframe index dropped) by the next
// library scan. Optional and chainable; without it the reindex still succeeds,
// the file is just re-probed later (a transcode window until keyframes
// backfill).
// WithActiveStreamChecker makes repair batches yield to live playback. Without
// it the service walks the library at full speed regardless of who is watching.
func (s *SeekIndexService) WithActiveStreamChecker(c ActiveStreamChecker) *SeekIndexService {
	s.activeStreams = c
	return s
}

// WithIdlePollInterval overrides how often a paused batch re-checks for idle
// playback. Mainly a test seam - production has no reason to move off the
// default.
func (s *SeekIndexService) WithIdlePollInterval(d time.Duration) *SeekIndexService {
	if d > 0 {
		s.idlePoll = d
	}
	return s
}

func (s *SeekIndexService) WithFileStateRefresher(r FileStateRefresher) *SeekIndexService {
	s.fileState = r
	return s
}

// WithAudioDelayProber wires the audio start-delay diagnosis into scans (one
// header-only ffprobe per file - the scan stays an async job with progress).
// Optional and chainable; without it AudioDelaySec stays 0 everywhere.
func (s *SeekIndexService) WithAudioDelayProber(p AudioDelayProber) *SeekIndexService {
	s.audioDelay = p
	return s
}

// WithAudioRemuxer wires the audio start-delay REPAIR (a validated remux).
// Optional and chainable; without it the remux entry points fail cleanly.
func (s *SeekIndexService) WithAudioRemuxer(r AudioDelayRemuxer) *SeekIndexService {
	s.audioRemux = r
	return s
}

// WithAudioTailProber wires the audio tail-gap diagnosis into scans: does the
// audio content actually reach the container's declared duration? Runs after
// the mkvgo triage (and is skipped for a wrong-container file, whose Matroska
// duration is meaningless). Optional and chainable; without it
// AudioShortfallSec stays 0 everywhere.
func (s *SeekIndexService) WithAudioTailProber(p AudioTailProber) *SeekIndexService {
	s.audioTail = p
	return s
}

// WithDiagnoser wires mkvgo's one-call triage (ops.Diagnose) into scans. When
// present it SUBSUMES the stacked HasSeekIndex + audio-delay probes with a
// single head-mostly read that additionally: catches a present-but-unusable
// index (audio-keyed or stale cues) as a defect, and classifies corruption as
// truncated (re-acquire the source) vs damaged (resync-repairable, N bytes
// lost) - the signal that routes the repair. Optional and chainable; without
// it scans fall back to the separate HasSeekIndex + AudioDelayProber probes.
func (s *SeekIndexService) WithDiagnoser(d mkv.Diagnoser) *SeekIndexService {
	s.diagnoser = d
	return s
}

// refreshFileState re-stamps the scan fingerprint (size+mtime) on a file a
// reindex just rewrote, so the next library scan sees it UNCHANGED and keeps
// the existing streams + keyframe index instead of re-probing (which nulls
// keyframes_ms and forces a transcode window). Best-effort: any failure only
// means the file is re-probed later - the pre-refresh behaviour - so it is
// logged, never fatal to the job. Runs right after the swap to beat the ~2 min
// periodic scan to the file.
func (s *SeekIndexService) refreshFileState(ctx context.Context, path string) {
	if s.fileState == nil {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		s.logger.Warn("seek-index: stat after reindex failed (file will be re-probed)",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	if err := s.fileState.RefreshFileState(ctx, path, fi.Size(), fi.ModTime()); err != nil {
		s.logger.Warn("seek-index: refresh file fingerprint failed (file will be re-probed)",
			slog.String("path", path), slog.Any("err", err))
	}
}

var ebmlExts = map[string]bool{".mkv": true, ".mka": true, ".mks": true, ".mk3d": true, ".webm": true}

func isEBMLPath(p string) bool { return ebmlExts[strings.ToLower(filepath.Ext(p))] }

func containerOf(p string) string { return strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".") }

// IsReadOnlyFS reports whether err ultimately comes from a read-only mount -
// the common hardening where media volumes are bound :ro in docker-compose.
// The string fallback covers error chains that didn't wrap with %w.
func IsReadOnlyFS(err error) bool {
	return errors.Is(err, syscall.EROFS) || strings.Contains(err.Error(), "read-only file system")
}

func jobKey(libraryID string) string {
	if libraryID == "" {
		return "*"
	}
	return libraryID
}
