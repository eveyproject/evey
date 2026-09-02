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
	// JobFixContainer renames .mkv files whose bytes are really MP4/MOV to
	// .mp4 and repoints the catalog: there is no Matroska index to build in
	// them, the extension is the defect.
	JobFixContainer = "fix_container"

	PhaseScanning   = "scanning"
	PhaseProcessing = "processing"
)

// maxFailures bounds the per-job failure list kept for the status endpoint.
const maxFailures = 50

// ScanPace is the operator's single choice when starting a scan: how much of
// the media disks the walk is allowed to take while it runs.
type ScanPace string

const (
	// ScanPaceGentle probes one file at a time and then stands aside for about
	// as long as that file took to read, so the disks are free roughly half the
	// time and a film keeps playing while the library is diagnosed. In prod the
	// scan reads three zones per file (head, cues near the end, tail): three
	// head moves that a player sharing the spindle loses every time. The
	// default, because nobody watches a scan and everybody watches their films.
	ScanPaceGentle ScanPace = "gentle"
	// ScanPaceFast probes several files at once and never pauses. It is for a
	// library spread over several disks (the prod pool is three ext4 disks
	// behind mergerfs), where one file at a time leaves two spindles idle and
	// the first full scan takes ~2h on 6695 files.
	ScanPaceFast ScanPace = "fast"
)

// ScanRunOptions carries the operator's scan choices from the API layer.
type ScanRunOptions struct {
	Pace ScanPace
}

// pace resolves the effective pace: anything but an explicit "fast" - unset,
// unknown, a value from an older client - is the gentle one. A wrong guess
// must cost a slow scan, never a starved player.
func (o ScanRunOptions) pace() ScanPace {
	if o.Pace == ScanPaceFast {
		return ScanPaceFast
	}
	return ScanPaceGentle
}

// scanGentleIdleRatio is how long the gentle pace idles per unit of time spent
// reading. 1 gives a 50 % duty cycle: measured, not guessed - the pause is
// derived from the file that was just probed, so a 4 GB remux earns a longer
// stand-aside than a 300 MB episode without anyone tuning a byte rate.
const scanGentleIdleRatio = 1.0

// scanFastWorkers bounds the fast pace. Sized for the prod pool (three disks
// behind mergerfs) plus one in flight while another waits on a head move; more
// workers than spindles only turns sequential reads into a seek storm.
const scanFastWorkers = 4

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

// AudioCoherenceProber answers the only question that turns a declared audio
// offset into a defect: do this file's OWN audio tracks tell the same story at
// the same instant? Two tracks of one film share their music and effects bed,
// so once their declared offsets are applied they line up - whatever those
// offsets are. A lag between them means one track sits in the wrong place.
//
// lagSec is the largest disagreement found between any two tracks (signed,
// seconds). confidence is the correlation strength behind it, 0..1: two dubs
// with different mixes, or a commentary track, can leave no common signal at
// all, and a peak read off noise must never accuse a file. Callers ignore any
// lag whose confidence is below AudioCoherenceMinConfidence.
//
// Returns lag 0 with confidence 0 when the file has a single audio track:
// there is nothing to compare, which is NOT the same as "proven coherent".
type AudioCoherenceProber interface {
	ProbeAudioTrackLagSec(ctx context.Context, path string) (lagSec, confidence float64, err error)
}

// AudibleTracksProber counts how many of a file's audio tracks actually carry
// SOUND - not how many are declared, which is what a structural verification
// already checks and what a silent track satisfies just as well.
//
// It exists because of what a structural check cannot see: on prod
// (2026-09-01) two films came back from an audio repair with their French
// track intact in every header and completely silent in every player. The
// rewrite had verified track count, duration and index, and passed.
//
// Returns the count of tracks with signal, or an error when the file could not
// be measured at all - the caller treats "could not measure" as no evidence,
// never as a loss. Satisfied by the ffmpeg adapter; optional (nil disables the
// guard).
type AudibleTracksProber interface {
	ProbeAudibleAudioTracks(ctx context.Context, path string) (int, error)
}

// ErrAudioLost is the refusal of a repair that cost the file its sound: the
// rewrite succeeded structurally, the backup has been put back, and the file
// is reported so the operator sees it rather than discovering it in a player.
var ErrAudioLost = errors.New("the repair left fewer audible audio tracks than the original")

// AudioCoherenceMinConfidence is the correlation strength below which a
// measured lag is discarded as noise. Measured on the real library: of 13
// files whose tracks appeared to disagree, 9 sat at r <= 0.36 and were simply
// dubs with different mixes; every genuine displacement scored r >= 0.71.
const AudioCoherenceMinConfidence = 0.5

// AudioDelayFlagSec is the floor above which a probed audio start delay is
// surfaced as a defect. Matches the HLS builders' cancel floor: legitimate
// codec priming sits at ~20-40ms, the defect class at hundreds of ms.
const AudioDelayFlagSec = 0.1

// RetimeSignature reports the fingerprint a track retime leaves in a file's
// declared audio start delays (ms, keyed by track): one track at EXACTLY 0
// while a sibling keeps a small non-zero offset under AudioDelayFlagSec, and no
// track above that floor. A muxer treats sibling tracks alike (both 0, or both
// carrying their priming); only a repair that pulled ONE track back by its own
// declared delay - the 2026-07-13 batch, which acted on every track above the
// floor - leaves exactly one at zero. Observed on the victims: track 2 at
// 0.000 s, track 3 at 0.020 s. A file whose tracks are ALL at zero is not
// flagged: that is what a plain mux looks like too, and the track comparison
// is what catches those.
func RetimeSignature(delaysMs map[uint64]int64) bool {
	if len(delaysMs) < 2 {
		return false
	}
	floorMs := int64(AudioDelayFlagSec * 1000)
	zeroed, kept := false, false
	for _, ms := range delaysMs {
		switch {
		case ms == 0:
			zeroed = true
		case ms > floorMs:
			// A track still above the floor was not retimed - the batch
			// zeroed every one of those. Not the shape it leaves behind.
			return false
		default:
			kept = true
		}
	}
	return zeroed && kept
}

// AudioTrackLagFlagSec is how far two of a file's own audio tracks may disagree
// before the file is called incoherent. Codec priming and frame quantisation
// leave tens of milliseconds between an AC-3 and an AAC rendition of the same
// soundtrack; a quarter of a second is far beyond that and already audible.
const AudioTrackLagFlagSec = 0.25

// AudioShortfallFlagSec is the floor above which an audio track ending before
// the declared duration is surfaced as a defect. Ordinary muxes leave up to a
// couple of seconds of slack (priming, final-cluster rounding); the defect
// class is measured in minutes (249 s on the file that motivated this).
const AudioShortfallFlagSec = 10.0

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
	// AudioDelaySec is how far after the picture the latest audio track starts,
	// as the CONTAINER declares it. This is a FACT, not a verdict: a dub is
	// aligned to a cut exactly this way, and every player honours it. Evey's own
	// pipeline cancels a spurious one at playback (-itsoffset). Only
	// AudioTracksLagSec can turn it into a defect. 0 when not probed.
	AudioDelaySec float64
	// AudioTracksLagSec is the PROOF: how far two of this file's own audio
	// tracks disagree once their declared offsets are applied. Two tracks of the
	// same film share their music and effects bed, so in a sound file they line
	// up (lag ~0) whatever offsets they declare. A non-zero lag means one track
	// sits in the wrong place - and since nothing here can tell WHICH, the file
	// is reported and nothing more, never an automatic rewrite. 0 = coherent, or the
	// file has a single audio track (nothing to compare), or not measured.
	AudioTracksLagSec float64
	// AudioTracksChecked records that the comparison actually ran, so "0" reads
	// as "proven coherent" instead of "never looked".
	AudioTracksChecked bool
	// AudioTracks is how many audio tracks the diagnosis saw. It decides
	// whether the coherence comparison has anything to compare: it used to run
	// only on files DECLARING an offset, and a file whose track has been pulled
	// back onto the picture declares none - the 98 files rewritten on
	// 2026-07-13 would all have passed as healthy. 0 when the diagnosis did not
	// count them (no diagnoser wired).
	AudioTracks int `json:"AudioTracks,omitempty"`
	// AudioTrackZeroed is the FINGERPRINT of a track that was retimed onto the
	// picture: one audio track starts at exactly 0 while another keeps its
	// small original offset (codec priming, a few tens of milliseconds). A
	// muxer gives sibling tracks the same treatment; only a repair that pulls
	// ONE track back by its own declared delay leaves this shape behind. It
	// says WHICH files were touched, for the price of a header read - what the
	// track comparison could establish on 4 of the 81 victims. It does not say
	// by how much: that value is gone from the file.
	AudioTrackZeroed bool `json:"AudioTrackZeroed,omitempty"`
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
	// (an informational verdict, see Verdict) can be acknowledged: a
	// repairable defect is repaired, not waved through. Carried by the
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
//	3: mkvgo 0.27 - the index tail is judged against the picture's real end (files
//	   cued every 2 s were condemned for sound outlasting picture), sparse holes are
//	   probed (reindex / re-encode / picture-missing), audio-short from TrackEnds
//	4: the audio track comparison runs on EVERY file with two or more audio
//	   tracks, not only on those declaring an offset (a retimed track declares
//	   none - none of the 98 files of 2026-07-13 would have been caught); a
//	   track pulled to exactly 0 next to a sibling keeping its priming offset is
//	   reported as the fingerprint of that retime
const SeekDiagVersion = 4

// Verdict is what the health page owes the operator for every file: not a
// pile of flags, but the ONE answer "does this need doing, or is it just
// something we noticed". TWO non-healthy answers, and deliberately only two:
//
//   - VerdictRepairable: Evey repairs it, here, now (index rebuild, resync of
//     a damaged stretch, container rename). "Repair all" acts on exactly
//     these, and its count is exactly their count.
//   - VerdictInformational: everything else. What we measured about the file,
//     offered as a remark and nothing more - possibly wrong, never a chore.
//     A too-sparse keyframe grid, an incomplete download, a shorter audio
//     track, an offset between the tracks, junk past the Segment end.
//
// There used to be a third, "your call": defects handed BACK to the operator
// with an instruction (re-encode this, re-acquire that). It was dropped on
// purpose. A detector that cannot repair something has not earned the right to
// give orders, and several of these measurements are demonstrably wrong on
// real files (audio-short false positives, prod 2026-08-31). Either we fix it,
// or we mention it - there is no third thing to ask of a person whose library
// plays fine.
//
// Informational verdicts can be acknowledged so the page tends towards zero
// instead of carrying the same rows for years.
type Verdict string

const (
	VerdictHealthy       Verdict = "healthy"
	VerdictRepairable    Verdict = "repairable"
	VerdictInformational Verdict = "informational"
)

// Verdict derives the file's single answer from its flags: can Evey act on it
// here, or is it a remark? A defect Evey cannot repair is NOT promoted above
// one it can - promoting it only made the page shout louder about the files
// nothing could be done for.
func (st SeekStatus) Verdict() Verdict {
	switch {
	// Repairing the index of an incomplete download is wasted work until the
	// source is re-acquired, so a truncated file stays out of the queue - as a
	// remark, not as an instruction.
	case st.Truncated, st.PictureMissing, st.SeekCoarse():
		return VerdictInformational
	case !st.HasIndex, st.DamagedBytes > 0, st.WrongContainer:
		return VerdictRepairable
	// An audio track that starts after the picture is NOT a defect to rewrite.
	// It is how a dub gets aligned to a cut, every player honours it, and Evey's
	// own pipeline already cancels a spurious one at playback (-itsoffset, since
	// 2026-07-08). Rewriting the file to zero it destroyed a DELIBERATE 12.000 s
	// alignment on a real title (a prod title, prod 2026-07-13): the dub then
	// played 12 s early in every player, and no backup existed. Measured
	// disagreement between the file's own tracks (AudioTracksLagSec) is worth
	// SAYING, because it is the one case where the offset is probably wrong -
	// but it is still not something Evey will rewrite behind the operator.
	case st.AudioTracksLagSec > AudioTrackLagFlagSec,
		st.AudioDelaySec > AudioDelayFlagSec,
		st.AudioTrackZeroed,
		st.AudioShortfallSec > AudioShortfallFlagSec,
		st.TrailingJunk:
		return VerdictInformational
	}
	return VerdictHealthy
}

// Acked reports whether the operator acknowledged this file's verdict.
func (st SeekStatus) Acked() bool { return st.AckedAt > 0 }

// Ackable reports whether the verdict is one the operator may acknowledge:
// only what nobody here can repair. A repairable defect gets repaired.
func (st SeekStatus) Ackable() bool { return st.Verdict() == VerdictInformational }

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
	// what "Repair all" will act on. Informational counts only the remarks
	// still AWAITING the operator's eyes: an acknowledged file moves to
	// Acked, whatever its verdict.
	Repairable    int
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
	fileState  FileStateRefresher   // optional; see WithFileStateRefresher
	audioDelay AudioDelayProber     // optional; see WithAudioDelayProber
	audioCoh   AudioCoherenceProber // optional; see WithAudioCoherenceProber
	audible    AudibleTracksProber  // optional; see WithAudibleTracksProber
	journal    RepairJournal        // optional; see WithRepairJournal
	retimer    TrackRetimer         // optional; see WithTrackRetimer
	diagnoser  mkv.Diagnoser        // optional; see WithDiagnoser
	relocator  MediaRelocator       // optional; see WithMediaRelocator
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

	// now and sleep are the scan pacer's only contact with time. Fields rather
	// than direct calls so a pacing test can drive a virtual clock and read the
	// pauses back instead of waiting them out. Always non-nil (constructor).
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration)

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
		now:       time.Now,
		sleep:     sleepCtx,
	}
}

// sleepCtx waits d, or until the job is cancelled - whichever comes first. A
// pause taken to spare the disks must never be a pause the operator has to sit
// through after pressing cancel.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
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

// WithAudioCoherenceProber wires the PROOF behind an audio offset: it decides
// whether a declared offset is the deliberate alignment of a dub (leave it
// alone) or a track sitting in the wrong place (worth mentioning). It is
// deliberately gated on the offset itself, so it runs on the handful of files
// that declare one - 3 of 6695 on the real library - and costs nothing on the
// rest. Optional and chainable; without it AudioTracksChecked stays false and
// no file is ever accused.
func (s *SeekIndexService) WithAudioCoherenceProber(p AudioCoherenceProber) *SeekIndexService {
	s.audioCoh = p
	return s
}

// WithAudibleTracksProber wires the guard that stands between a repair and a
// silent file: every rewrite is measured before and after, and one that costs
// the file an audible track is undone from its own backup. Optional and
// chainable; without it a rewrite is verified structurally only, which is
// exactly what let a silenced track through in prod (see AudibleTracksProber).
func (s *SeekIndexService) WithAudibleTracksProber(p AudibleTracksProber) *SeekIndexService {
	s.audible = p
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
