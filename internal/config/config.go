package config

import (
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// validateCosignPublicKey checks that the configured cosign public key path
// points to a readable PEM-encoded public key. Run at startup so a typo or
// missing file is surfaced immediately rather than at the first plugin
// install. Accepts standard PEM blocks (PUBLIC KEY) only; cosign also
// supports key references prefixed with k8s://, gcpkms://, etc., which we
// treat as opaque and skip validation for.
func validateCosignPublicKey(pathOrRef string) error {
	if strings.Contains(pathOrRef, "://") {
		// remote KMS reference - cosign resolves at verify time.
		return nil
	}
	data, err := os.ReadFile(pathOrRef)
	if err != nil {
		return fmt.Errorf("cannot read public key file %q: %w", pathOrRef, err)
	}
	if len(data) == 0 {
		return fmt.Errorf("public key file %q is empty", pathOrRef)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("public key file %q is not a PEM-encoded key", pathOrRef)
	}
	if !strings.Contains(block.Type, "PUBLIC KEY") {
		return fmt.Errorf("public key file %q has unexpected PEM type %q (expected PUBLIC KEY)", pathOrRef, block.Type)
	}
	return nil
}

type Config struct {
	// mu guards the handful of fields the admin "server-settings" PUT mutates at
	// runtime (Server.Name, Server.{Login,Global}RateLimitMax,
	// Transcoding.{MaxConcurrent,HWAccelEnabled,HWMode,AV1Enabled,AV1MaxHeight,
	// SoftwareAV1MaxHeightOverride}, Log.Level, Library.{WatcherEnabled,
	// PeriodicScanInterval}, Metadata.SkipPreviewEnabled) against the live
	// readers in the request path. Writers go through ApplyServerSettings; live
	// readers go through the Effective* accessors. Everything else is set once at
	// boot and never written again, so it needs no lock.
	mu sync.RWMutex

	Server       ServerConfig
	DB           DBConfig
	Meilisearch  MeilisearchConfig
	JWT          JWTConfig
	Transcoding  TranscodingConfig
	OIDC         OIDCConfig
	Metrics      MetricsConfig
	Library      LibraryConfig
	Playback     PlaybackConfig
	Metadata     MetadataConfig
	Log          LogConfig
	Backup       BackupConfig
	RemoteAccess RemoteAccessConfig
	SMTP         SMTPConfig
	VAPID        VAPIDConfig
	Tracing      TracingConfig
	Plugins      PluginsConfig
}

// PluginsConfig holds configuration for the third-party plugin subsystem.
type PluginsConfig struct {
	// SandboxMode controls the isolation level applied to plugin processes.
	// Valid values:
	//   "auto"       detect the strongest isolation the host supports at boot
	//                (landlock > bubblewrap > basic) - recommended, works OOTB.
	//   "landlock"   Landlock LSM filesystem allowlist; no mount/unshare, so it
	//                runs under Docker's default seccomp+AppArmor. Fail-closed.
	//   "bubblewrap" bwrap namespace isolation (needs the mount syscall, so it
	//                only works on bare-metal / relaxed-security hosts).
	//   "strict"     bubblewrap + LD_PRELOAD seccomp.
	//   "basic"      LD_PRELOAD seccomp + UID drop only (no per-path FS isolation).
	// Defaults to "auto" when unset or empty.
	// EVEY_PLUGINS_SANDBOX_MODE
	SandboxMode string

	// Per-plugin resource caps enforced in userspace by the runner's resource
	// monitor (cgroups are unusable in a non-privileged Docker container, so a
	// background sweeper polls /proc and stops a plugin that exceeds its caps).
	// Zero values use the runner package defaults (256 MiB, 64 PIDs).
	// EVEY_PLUGINS_MEMORY_MB - max RSS in MiB, default 256.
	MemoryMB int64
	// EVEY_PLUGINS_CPU_NICE - nice value (0-19) applied to each plugin process at
	// spawn so it can never starve the server of CPU under contention, default 10.
	// 0 leaves the OS-default priority untouched.
	CPUNice int
	// EVEY_PLUGINS_PIDS_MAX - max process/thread count per plugin, default 64.
	PIDsMax int
	// EVEY_PLUGINS_DISK_MAX_MB - max install-directory size per plugin in MiB,
	// default 100. The runner measures the dir and stops a plugin that grows past
	// this. 0 disables the check.
	DiskMaxMB int

	// UIDSeparation controls running each plugin under the dedicated evey-plugin
	// uid (1001), distinct from the server's, so plain DAC keeps the server's
	// 0600 secrets (jwt.key, backups) unreadable to plugins on a host without
	// Landlock. Values:
	//   "auto" (default) - enable only when the sandbox resolves to "basic"
	//                      (neither Landlock nor bubblewrap isolates the FS);
	//                      skipped under landlock/bubblewrap where it's redundant.
	//   "on"             - always attempt it.
	//   "off"            - never.
	// Where the launcher can't setuid (no cap_setuid file capability, or
	// no-new-privileges) a basic-mode plugin is refused; under landlock it runs
	// same-uid with a loud warning. EVEY_PLUGINS_UID_SEPARATION
	UIDSeparation string

	// CosignPolicy controls whether plugin ZIPs must carry a cosign signature.
	// Values: "off" (no verification), "warn" (default - install proceeds but
	// unsigned/invalid plugins surface a finding in the security dashboard),
	// "enforce" (block install when signature is missing or invalid).
	// EVEY_PLUGINS_COSIGN_POLICY
	CosignPolicy string
	// CosignPublicKey is a path to the PEM public key used to verify signatures.
	// Required when CosignPolicy != "off".
	// EVEY_PLUGINS_COSIGN_PUBLIC_KEY
	CosignPublicKey string
}

type TracingConfig struct {
	OTLPEndpoint string  // EVEY_OTLP_ENDPOINT, empty = disabled
	ServiceName  string  // EVEY_TRACING_SERVICE_NAME, default "evey"
	SampleRatio  float64 // EVEY_TRACING_SAMPLE_RATIO in [0,1], default 0
}

type MetadataConfig struct {
	Language string // ISO 639-1, default "en"

	// IntroProvidersEnabled wires external segment providers (TIDB +
	// IntroDB) ahead of the in-house intro detection pipeline. Off by
	// default - outbound HTTP is opt-in, the user must consent.
	// EVEY_INTRO_PROVIDERS_ENABLED
	IntroProvidersEnabled bool

	// TIDBAPIKey is an optional Bearer token for theintrodb.org. Without
	// it the daily quota is per-IP (lower); with it the quota lifts to
	// per-account 500/day for /media. Treat as a secret.
	// EVEY_INTRO_TIDB_API_KEY
	TIDBAPIKey string

	// SkipPreviewEnabled controls whether "preview" (next-episode preview
	// reel) segments are served to the player. Off by default - the admin
	// opts in from the panel. Read live per request by the segments handler,
	// so a change takes effect without a restart.
	// EVEY_INTRO_SKIP_PREVIEW_ENABLED
	SkipPreviewEnabled bool

	// IntroAutoSkipEnabled controls automatic intro skipping on EVERY player.
	// Cast receivers auto-jump whatever intro segments they receive, so the
	// segments handler withholds them when this is off; web/mobile players
	// read the flag in the segments response and auto-skip when it is on
	// (their manual skip button stays regardless). Off by default - the
	// admin opts in from the panel. Read live per request, no restart.
	// EVEY_INTRO_AUTO_SKIP_ENABLED
	IntroAutoSkipEnabled bool
}

type VAPIDConfig struct {
	PublicKey  string // VAPID_PUBLIC_KEY
	PrivateKey string // VAPID_PRIVATE_KEY
	Subject    string // VAPID_SUBJECT
}

type SMTPConfig struct {
	Host     string // EVEY_SMTP_HOST
	Port     int    // EVEY_SMTP_PORT, default 587
	Username string // EVEY_SMTP_USERNAME
	Password string // EVEY_SMTP_PASSWORD
	From     string // EVEY_SMTP_FROM
	TLS      string // EVEY_SMTP_TLS: none, starttls, ssl
}

type BackupConfig struct {
	Dir           string
	IntervalHours int // 0 = disabled
	RetentionDays int // 0 = keep forever
	// IncludeTelemetry opts routine backups into dumping the high-volume
	// telemetry tables. Off by default: they are regenerable and bloat every
	// backup, so a lean snapshot excludes their data (the rows are still cleared
	// on restore, never reloaded).
	IncludeTelemetry bool
}

type RemoteAccessConfig struct {
	Enabled bool
	// EnabledExplicit records that Enabled is an OPERATOR CHOICE rather than a
	// default: the env var was set, or the panel saved it. ResolveRemoteAccess
	// never overrides a choice - it only fills in the blank when nobody made
	// one. Without this flag "false" is ambiguous (chosen, or merely never
	// touched), and that ambiguity is what locked every remote user out of a
	// published instance on 2026-07-29. Persisted with the rest of the struct.
	EnabledExplicit      bool
	ExternalURL          string
	LANNetworks          []string
	MaxRemoteBitrateKbps int
	// TrustedProxies is the list saved in Admin -> Remote Access. When not
	// empty it replaces EVEY_TRUSTED_PROXIES (loopback is always kept, see
	// NormalizeTrustedProxies); empty hands the answer back to the environment.
	TrustedProxies []string
}

type LogConfig struct {
	Level               string
	Dir                 string
	MaxSizeMB           int
	RetentionDays       int
	PluginRetentionDays int
	AuditRetentionDays  int // DB audit_log table; 0 = keep forever (append-only)
	DevMode             bool
}

type ServerConfig struct {
	Port int
	// GRPCBindAddr is the interface the NATIVE gRPC listener binds to,
	// "127.0.0.1" by default. It is an INTERNAL port: the only client is this
	// same process's grpc-gateway, which dials it over loopback, and the
	// shipped compose file publishes just the HTTP port.
	//
	// It defaults to loopback because the remote-access gate - the switch that
	// decides whether anyone outside the LAN may reach this server - is HTTP
	// middleware and does NOT cover this listener. Binding it to every
	// interface therefore offered a way around the gate that no operator was
	// warned about and no code documented. Set EVEY_GRPC_BIND=0.0.0.0 to
	// expose it deliberately; boot then WARNS that the gate does not apply.
	GRPCBindAddr       string
	BaseURL            string
	Name               string
	WebDir             string
	TrustedProxies     []string
	RealIPHeader       string
	ForwardedProto     bool
	LoginRateLimitMax  int
	GlobalRateLimitMax int
	// PluginsEnabled gates the entire third-party plugin subsystem.
	// Defaults to true: plugins are a first-class feature and always run inside
	// the plugin sandbox. "auto" picks the strongest the host supports
	// (landlock > bubblewrap > basic); even the basic fallback applies a UID
	// drop + seccomp denylist, so a plugin never runs fully unconfined.
	// Installing remains admin-only and gated by the analyzer + cosign +
	// permission ack. Set EVEY_PLUGINS_ENABLED=false to disable the subsystem
	// entirely (admin RPCs return FailedPrecondition, the runner is not
	// constructed).
	PluginsEnabled bool
	// BenchInstance runs this server as a load-test bench beside a
	// production one (EVEY_BENCH_INSTANCE, deliberately undocumented): every
	// background task is off - scans, re-probes, trickplay, intro detection,
	// calibration, backfills, search reindex, backups, notifications, plugins -
	// so the only work on the disks is the load test's. Meant for a second
	// container on a copy of the production database, media mounted read-only.
	BenchInstance bool
	// Demo turns this instance into a public demo: every visitor is an admin,
	// so every admin operation is refused except the reads named in
	// internal/pkg/demo, and the outbound ports (mail, webhooks, push) are
	// wired to sinks. Read once from EVEY_DEMO at boot, never persisted and
	// never shown by the panel - a switch a visitor could flip is not a switch.
	// The one deliberate exception to "operator settings live in the panel".
	Demo bool
	// DemoExtended is the second level of the demo (EVEY_DEMO=extended): the
	// same sinks, seed and redaction as Demo, but a private instance for a
	// outside evaluator, so almost every write is open (internal/pkg/demo,
	// ExtendedStaysLocked names what stays shut). Always implies Demo.
	DemoExtended bool
	// DemoUser and DemoPassword are the one administrator of the extended
	// demo, set by the operator (EVEY_DEMO_USER, EVEY_DEMO_PASSWORD) and given
	// to the outside evaluator out of band. Validated at boot: the user is
	// letters and digits, the password at least 12 printable characters
	// without a space. Empty on the public demo, which publishes demo / demo.
	DemoUser     string
	DemoPassword string
	// DemoMediaDir is where the demo bootstrap downloads the freely licensed
	// media and points its libraries. Read once with Demo.
	DemoMediaDir string
	// DemoPluginDir holds the plugin bundles the demo bootstrap installs
	// (internal/pkg/demo.Plugins); a bundle that is not there is skipped.
	DemoPluginDir string
	// PasswordRecoveryEnabled mirrors the panel's password-recovery switch
	// (server_config auth.password_recovery_enabled), for the settings GET.
	// The recovery flow itself reads the stored key on every request, and
	// still needs SMTP.
	PasswordRecoveryEnabled bool
}

type DBConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	DBName       string
	SSLMode      string
	MaxOpenConns int
	MaxIdleConns int
	// AppUser is the unprivileged role the app and its migrations run as when
	// User is a superuser; User is then kept for the restore only.
	// EVEY_DB_APP_USER, default evey_app.
	AppUser string
	// PrivilegeSplit turns that split off (EVEY_DB_PRIVILEGE_SPLIT=false): the
	// escape hatch if a database refuses it, at the cost of a superuser app.
	PrivilegeSplit bool
}

func (c DBConfig) DSN() string {
	// libpq keyword/value format - values containing spaces, quotes or
	// backslashes are wrapped in single quotes with ' and \ escaped.
	// Without this, a password like `p@ss 'foo` silently breaks the DSN.
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		escapeDSNValue(c.Host),
		c.Port,
		escapeDSNValue(c.User),
		escapeDSNValue(c.Password),
		escapeDSNValue(c.DBName),
		escapeDSNValue(c.SSLMode),
	)
}

func escapeDSNValue(v string) string {
	needsQuote := v == ""
	for _, r := range v {
		if r == ' ' || r == '\'' || r == '\\' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return v
	}
	buf := make([]byte, 0, len(v)+2)
	buf = append(buf, '\'')
	for _, r := range v {
		switch r {
		case '\'', '\\':
			buf = append(buf, '\\', byte(r))
		default:
			buf = append(buf, string(r)...)
		}
	}
	buf = append(buf, '\'')
	return string(buf)
}

type MeilisearchConfig struct {
	URL    string
	APIKey string
}

type JWTConfig struct {
	PrivateKeyPath      string
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	RefreshTokenLongTTL time.Duration
}

type TranscodingConfig struct {
	MaxConcurrent int
	// MaxSessions caps the TOTAL concurrent HLS-VOD ffmpeg sessions the
	// orchestrator runs (MaxActiveSessions). 0 = runtime.NumCPU(). A weak NAS
	// that can barely sustain ONE 4K HDR software transcode must set this to 1-2
	// (EVEY_TRANSCODING_MAX_SESSIONS) or concurrent sessions starve each other.
	// Distinct from MaxConcurrent, which gates the legacy (non-VOD) service.
	MaxSessions    int
	CacheMaxGB     int
	CacheDir       string
	HWAccelEnabled bool
	// HWBackend pins which HW encode backend this instance uses ("nvenc",
	// "vaapi", "qsv"); empty = auto-detect. A deployment-time knob: it lets one
	// host run several Evey containers, each bound to a different GPU path,
	// without the shared capability cache leaking one container's backend into
	// another. Honoured by calibration (only the pinned backend is benched, and
	// a persisted capability for a different backend is re-benched).
	HWBackend string
	// HWDevice pins the calibration to ONE device: a DRM render node
	// ("/dev/dri/renderD129") or a CUDA index ("1"). Empty = every detected
	// device is certified and the best per backend selected. With HWBackend it
	// is what binds one Evey container to one GPU on a multi-GPU host
	// (EVEY_TRANSCODING_HW_DEVICE).
	HWDevice string
	// HWMode is the admin-selectable HW-acceleration policy ("auto"/"off"/
	// "nvenc"/"vaapi"/"qsv"). Empty = derive the bootstrap mode from
	// HWAccelEnabled + HWBackend. The admin panel persists an override into
	// server_settings (transcoding.hw_mode) which loadOverride layers on top,
	// and changing it re-benches live via CalibrationService.SetHWMode.
	HWMode string
	// HWFirstSegmentTimeout is the HW fast-fail watchdog window: a hardware
	// transcode session that hasn't written its init segment within this delay
	// is treated as a dead/stuck GPU and demoted to software immediately, rather
	// than letting the player wait out the full segment timeout. Default 10s.
	HWFirstSegmentTimeout time.Duration
	// HWPerVariant, when true, makes the HW transcode path spawn one single-
	// output ffmpeg per ABR rung (one-encoder-per-rung model) instead of a single
	// multi-output process. Avoids the multi-encode-in-one-process graph that
	// crashes fragile VA drivers and isolates per-rung failures. Off by default;
	// software always uses the single-process multi-variant graph regardless.
	HWPerVariant bool
	// SingleVariant, when true, makes the VOD orchestrator encode only the
	// one ABR rung the client is requesting (1 decode + 1 encode, on-demand)
	// instead of one ffmpeg producing the whole ladder.
	//
	// ON by default. Multi-variant shares one decode across the rungs, which
	// reads well on paper and is ruinous in practice: it also encodes every rung
	// the viewer will never watch. Measured on prod (2026-09-08, a 2160p HEVC HDR
	// remux): ONE ffmpeg carrying `split=4` into four concurrent libx264 outputs
	// - 480p, 720p, 1080p AND a full 3840-wide rung at maxrate 24000k - while the
	// player was pinned to a single rung. It delivered no segment at all in 89 s
	// and the viewer sat on `fps=0.0/24.9`. Three of those four encodes were pure
	// waste, and they are what starved the fourth.
	//
	// The historic argument for multi-variant was that single-variant RE-DECODES
	// the source per rung, so a decode-bound host thrashes when the player probes
	// across rungs. That holds only while the decode is on the CPU: with a
	// calibrated HW decoder (HWDecodeCodecs) a 2160p HEVC decode is near-free, and
	// the wasted encodes dominate by far. On a host with no HW decode AND a heavy
	// source, set EVEY_TRANSCODING_SINGLE_VARIANT=0 to get the shared decode back.
	//
	// The segment cache is keyed (hash, variantIdx, segmentIdx) in both modes, so
	// flipping is cache-compatible.
	SingleVariant bool
	// SoftwareMaxHeightOverride / SoftwareHDRMaxHeightOverride force the software
	// encode ceilings, bypassing the boot benchmark (0 = use the calibrated
	// value). EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT / _SOFTWARE_HDR_MAX_HEIGHT.
	// For an operator to empirically push a higher HDR rung on a host where the
	// calibrated cap (1.2x realtime margin, and decode-bound on 4K HEVC10) pinned
	// it too low - e.g. set _SOFTWARE_HDR_MAX_HEIGHT=720 to try 720p HDR. The
	// runtime throttle + input pacing still protect a host that overshoots.
	SoftwareMaxHeightOverride    int
	SoftwareHDRMaxHeightOverride int
	// SoftwareAV1MaxHeightOverride forces the AV1 software ceiling (0 = use the
	// calibrated value; AV1 stays hardware-only when the bench found it non-viable).
	// Unlike the two above this is NOT an env knob - it is admin-settable from the
	// server-config panel (persisted under transcoding.software_av1_max_height,
	// loaded at boot via loadOverride and applied live by CalibrationService).
	SoftwareAV1MaxHeightOverride int
	// AV1Enabled gates AV1 as a transcode OUTPUT codec. AV1 is hardware-only for
	// streaming (libsvtav1 starves a weak CPU), so this is an explicit opt-in,
	// meaningful only when HW acceleration is on and the GPU can encode AV1.
	// Default false: a profile that prefers AV1 still transcodes to H.264 until
	// an admin turns this on. Persisted under transcoding.av1_enabled.
	AV1Enabled bool
	// AV1MaxHeight caps the AV1 output ladder height when AV1Enabled (0 = no cap,
	// up to the source/GPU ceiling). Lets an admin keep AV1 at e.g. 1080p even on
	// a GPU that could push 2160p. Persisted under transcoding.av1_max_height.
	AV1MaxHeight int
	// HWLowPowerEnabled opts into the Intel low-power (VDEnc) encoder when the
	// GPU probed it. Persisted under transcoding.hw_low_power. Default false.
	HWLowPowerEnabled bool
	// MaxVideoBitrateKbps caps the served video bitrate; 0 follows the source
	// bitrate (the default). Persisted under transcoding.max_video_bitrate_kbps.
	MaxVideoBitrateKbps int
	// MaxVideoBitrateRemoteKbps is the ceiling for a client outside the LAN
	// (public IP). 0 = NO remote-specific cap: the remote client follows the
	// LAN ceiling / the source, which is the DEFAULT - a server must not be
	// throttled to an arbitrary rate it never chose (a 70 Mb/s remux is fine on
	// the LAN and heavy over an uplink, but capping it is the OPERATOR's call,
	// not a silent default). 20000 / 40000 are the opt-in presets. A stricter
	// per-user limit still wins downstream (tighten()). An admin who explicitly
	// persisted a value keeps it - only the ABSENT default changed from 20000 to
	// 0. transcoding.max_video_bitrate_remote_kbps.
	MaxVideoBitrateRemoteKbps int
	TrickplayCacheDir         string
	// AudioRenditionEnabled exposes source audio tracks as HLS-native
	// renditions (#EXT-X-MEDIA TYPE=AUDIO) so players can switch language
	// without re-fetching the video stream.
	//
	// ON by default: it gates the HYBRID on-demand rung - a copy-eligible video
	// packaged by mkvgo (no ffmpeg, nothing written to the cache) while its
	// non-browser audio (AC-3/E-AC-3, very common) is re-encoded on the side. This
	// is the biggest win on a weak NAS, and the common case for a browser.
	//
	// The A/V desync this once caused is fixed: served as its own stream the audio
	// started at 0 while the video is presented late by has_b_frames/fps (MP4
	// forbids a negative DTS) - 42 ms for 1 B-frame, 83 ms for 2, past the ~45 ms
	// threshold for EARLY audio. The audio session now offsets its output by that
	// delay (VideoCompositionDelay). Opt out: EVEY_HLS_AUDIO_RENDITION_ENABLED=false.
	AudioRenditionEnabled bool

	// CastWasmAudioEnabled lets a CAST receiver run the browser wasm AC-3
	// decoder. OFF by default: a low-end operator box was killed twice mid-film with that
	// path in play (2026-07-30), and its decoder measured 3.13x real time, so the
	// path bought it nothing - the AAC rendition plays the same film with sound.
	// Browsers are unaffected; only a cast stream token is gated.
	CastWasmAudioEnabled bool

	// CopyFallbackRungEnabled advertises ONE low transcoded rung next to the
	// copy variant of the hybrid on-demand master, giving a player whose
	// throughput collapses an ABR escape below the source bitrate (the copy
	// path is otherwise single-variant: a bad home link freezes the picture
	// for minutes instead of degrading it - observed on a cast session,
	// 2026-07-31). Costs nothing until a player actually drops to it: only
	// then does a software encode session spawn, gated by the normal
	// admission machinery. EVEY_HLS_COPY_FALLBACK_RUNG=false to opt out.
	CopyFallbackRungEnabled bool

	// Encode-calibration bench sample sources. The bench measures the host's
	// realtime encode ceiling against a 4K clip; a real-content clip calibrates
	// far more honestly than the synthetic (which over-states decode+encode
	// cost and pins the cap low). Resolution order: operator file > pinned
	// download > synthetic. Empty values keep the synthetic fallback.
	//   *Path   a real file the operator dropped in (used verbatim)
	//   *URL    a license-clean reference clip, https, SHA256-pinned
	//   *SHA256 required when the matching *URL is set
	// EVEY_BENCH_SDR_SAMPLE / _URL / _SHA256
	BenchSDRSamplePath   string
	BenchSDRSampleURL    string
	BenchSDRSampleSHA256 string
	// EVEY_BENCH_HDR_SAMPLE / _URL / _SHA256
	BenchHDRSamplePath   string
	BenchHDRSampleURL    string
	BenchHDRSampleSHA256 string
}

type OIDCConfig struct {
	Enabled             bool
	Issuer              string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	AllowedEmailDomains []string // EVEY_OIDC_ALLOWED_EMAIL_DOMAINS - comma-separated; empty = allow all
	// AllowSignup is EVEY_OIDC_ALLOW_SIGNUP raw ("true"/"false"); empty keeps
	// the default (on only behind a domain allowlist). The admin panel wins.
	AllowSignup string
}

type MetricsConfig struct {
	RetentionDays     int
	SampleIntervalSec int
	// QoERetentionDays bounds playback_qoe_events. Defaults to the 30 days
	// migration 000103 sized its indexes for; that sweep was never written, so
	// the table grew unbounded on the server's busiest write path (one row per
	// CMCD-carrying segment request, JSONB payload, four indexes).
	// 0 = keep forever.
	QoERetentionDays int
	// ScrapeToken, when set, is an alternate static bearer accepted on the
	// GET /metrics endpoint in addition to the admin JWT. Lets a Prometheus
	// or Grafana datasource scrape without refreshing a ~15-min JWT every
	// scrape interval. Empty (the default) disables this path - only the
	// admin JWT is then accepted. Rotate by changing the env + restart.
	ScrapeToken string
}

type LibraryConfig struct {
	PeriodicScanInterval time.Duration // fallback polling when inotify misses events
	WatcherEnabled       bool
	// SeekIndexStateDir persists the admin seek-index scan cache and the
	// running-job marker across restarts (a full scan is minutes of per-file
	// probes; an in-memory job dies silently with the process). Empty
	// disables persistence.
	SeekIndexStateDir string
}

type PlaybackConfig struct {
	SkipForwardSeconds  int
	SkipBackwardSeconds int
	// SessionRetentionDays bounds the play_sessions table by deleting finished
	// sessions older than this. 0 = keep forever (default; play_sessions is the
	// admin session history, so purging is opt-in). Recommended ~90 for a small
	// self-hosted server that doesn't need deep history.
	SessionRetentionDays int
}

// EffectiveBaseURL is the public address in force now: the External URL saved
// in Admin -> Remote Access, else the base URL. Not to be called under the
// config lock.
func (c *Config) EffectiveBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RemoteAccess.ExternalURL != "" {
		return c.RemoteAccess.ExternalURL
	}
	return c.Server.BaseURL
}

// DefaultServerName is the instance name when the admin never picked one:
// what /healthz, the setup-status endpoint and the mails report, and what the
// clients treat as "unnamed" (a server list shows the URL instead).
const DefaultServerName = "Evey"

// The Effective* accessors below are the read side of the runtime-mutable
// settings guarded by c.mu. They take the read lock so a concurrent admin PUT
// (ApplyServerSettings) can't tear a value out from under a live reader.
func (c *Config) EffectiveServerName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if strings.TrimSpace(c.Server.Name) == "" {
		return DefaultServerName
	}
	return c.Server.Name
}
func (c *Config) EffectiveMaxConcurrent() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Transcoding.MaxConcurrent
}
func (c *Config) EffectiveHWAccelEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Transcoding.HWAccelEnabled
}

// EffectiveBackupIncludeTelemetry reports whether backups should dump the
// telemetry tables (read live so a panel/CLI change applies to the next backup).
func (c *Config) EffectiveBackupIncludeTelemetry() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Backup.IncludeTelemetry
}
func (c *Config) EffectiveAV1Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Transcoding.AV1Enabled
}
func (c *Config) EffectiveMaxVideoBitrateKbps() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Transcoding.MaxVideoBitrateKbps
}

// EffectiveMaxVideoBitrateRemoteKbps is the ceiling for a client outside the
// LAN: the stricter of Transcoding's remote ceiling and Remote Access's Max
// Remote Bitrate. Stream info used to apply only the second and the served
// master only the first, so a player was announced an 8 Mb/s transcode while
// the full-rate copy went out.
func (c *Config) EffectiveMaxVideoBitrateRemoteKbps() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return RemoteBitrateCapKbps(c.Transcoding.MaxVideoBitrateRemoteKbps, c.RemoteAccess.MaxRemoteBitrateKbps)
}

// RemoteBitrateCapKbps combines the two remote ceilings: 0 means "no cap"
// for either, otherwise the lower one wins.
func RemoteBitrateCapKbps(transcodingKbps, remoteAccessKbps int) int {
	switch {
	case transcodingKbps <= 0:
		return max(remoteAccessKbps, 0)
	case remoteAccessKbps <= 0:
		return transcodingKbps
	default:
		return min(transcodingKbps, remoteAccessKbps)
	}
}
func (c *Config) EffectiveHWLowPowerEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Transcoding.HWLowPowerEnabled
}
func (c *Config) EffectiveAV1MaxHeight() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Transcoding.AV1MaxHeight
}

// EffectiveRemoteAccessEnabled reports whether clients outside the LAN are
// allowed in. The remote-access gate reads it on EVERY request while an admin
// PUT can flip it at any moment - the one runtime-mutable setting that had kept
// a raw *bool handed to the middleware instead of an accessor, so the hot path
// read it with no lock at all while two handlers wrote it under two DIFFERENT
// private mutexes (they did not even synchronize with each other).
func (c *Config) EffectiveRemoteAccessEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.RemoteAccess.Enabled
}

// EffectivePlaybackSkipSeconds returns the forward/backward skip steps as one
// consistent pair - the same twin shape as the remote-access flag: two handlers
// (REST + gRPC) mutate them under two unrelated mutexes.
func (c *Config) EffectivePlaybackSkipSeconds() (forward, backward int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Playback.SkipForwardSeconds, c.Playback.SkipBackwardSeconds
}

// EffectiveSkipPreviewEnabled reads the intro-provider skip-preview toggle under
// the read lock. The segments handler calls it live per request, so it must not
// race the admin PUT that flips Metadata.SkipPreviewEnabled.
func (c *Config) EffectiveSkipPreviewEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Metadata.SkipPreviewEnabled
}

// EffectiveIntroAutoSkipEnabled reads the intro auto-skip toggle under the
// read lock; live per request like the skip-preview one.
func (c *Config) EffectiveIntroAutoSkipEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Metadata.IntroAutoSkipEnabled
}

// The retention accessors below are read live by the background purge sweeps
// (activity daily sweep, playback cleanup loop), so an admin change from the
// panel takes effect at the next sweep without a restart - the reason they go
// through the lock instead of a boot snapshot.

func (c *Config) EffectiveAuditRetentionDays() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Log.AuditRetentionDays
}

func (c *Config) EffectiveMetricsRetentionDays() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Metrics.RetentionDays
}

func (c *Config) EffectiveQoERetentionDays() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Metrics.QoERetentionDays
}

func (c *Config) EffectiveSessionRetentionDays() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Playback.SessionRetentionDays
}

// ApplyServerSettings runs fn while holding the write lock that guards the
// runtime-mutable settings fields. fn must touch only those fields (see the
// c.mu doc) and must not call back into the Effective* accessors (the lock is
// not reentrant). The admin server-settings PUT calls this once per field it
// changes, keeping the critical section to a plain struct assignment.
func (c *Config) ApplyServerSettings(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn()
}

// WithReadLock runs fn under the read lock guarding the runtime-mutable fields.
// Used by the settings GET handler to take a consistent snapshot of those raw
// fields without recursively locking (it reads fields directly, not via the
// Effective* accessors, which would deadlock under a nested RLock).
func (c *Config) WithReadLock(fn func()) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fn()
}

// WarnValidate validates config, returning an error for critical security issues
// and logging warnings for non-critical ones.
// In DevMode the hard errors are downgraded to warnings so dev iteration stays smooth.
func (c *Config) WarnValidate() error {
	dev := c.Log.DevMode
	var errs []string

	hardFail := func(msg string) {
		if dev {
			slog.Warn("config hardening", slog.String("issue", msg), slog.Bool("dev_mode", true))
			return
		}
		errs = append(errs, msg)
	}

	if c.DB.Password == "evey" {
		hardFail("EVEY_DB_PASSWORD is the default 'evey' - set a strong database password")
	}
	// Warned, not refused: installs that copied .env.example would stop on
	// their next upgrade, for a database the compose keeps off the network.
	if c.DB.Password == "changeme" {
		slog.Warn("config hardening: EVEY_DB_PASSWORD is the .env.example placeholder 'changeme' - set a strong database password")
	}

	// Finite ranges. Out of 98 EVEY_* variables only the plugin enums were
	// checked, so a value that cannot possibly work started the server anyway
	// and failed later somewhere unrelated. These three each have a concrete
	// failure story rather than a theoretical bound.
	for _, p := range []struct {
		key string
		val int
	}{
		{"EVEY_SERVER_PORT", c.Server.Port},
		{"EVEY_DB_PORT", c.DB.Port},
		{"EVEY_SMTP_PORT", c.SMTP.Port},
	} {
		// 0 is left alone: it means "unset/disabled" for SMTP and "pick a free
		// port" for a listener, both legitimate.
		if p.val < 0 || p.val > 65535 {
			slog.Warn("config: port outside the valid range - the listener or dial will fail",
				slog.String("key", p.key), slog.Int("value", p.val), slog.String("expected", "0-65535"))
		}
	}

	// database/sql silently caps idle at open, so the operator gets a pool
	// smaller than the one they configured, with nothing said.
	if c.DB.MaxIdleConns > c.DB.MaxOpenConns && c.DB.MaxOpenConns > 0 {
		slog.Warn("EVEY_DB_MAX_IDLE_CONNS is greater than EVEY_DB_MAX_OPEN_CONNS - database/sql caps idle at open",
			slog.Int("idle", c.DB.MaxIdleConns), slog.Int("open", c.DB.MaxOpenConns))
	}

	// A ratio, not a percentage: "50" meaning "50%" samples every trace.
	if c.Tracing.SampleRatio < 0 || c.Tracing.SampleRatio > 1 {
		slog.Warn("EVEY_TRACING_SAMPLE_RATIO is outside 0.0-1.0 - it is a ratio, not a percentage",
			slog.Float64("value", c.Tracing.SampleRatio))
	}

	if c.DB.SSLMode == "prefer" && !isLoopbackHost(c.DB.Host) {
		slog.Warn("EVEY_DB_SSLMODE=prefer against a non-loopback host - use 'require' or 'verify-full' in production",
			slog.String("host", c.DB.Host))
	}

	k := c.Meilisearch.APIKey
	if k == "" || k == "changeme" || k == "change-me-generate-a-random-secret" {
		hardFail("MEILI_MASTER_KEY is empty or default - set a strong secret (>= 16 chars)")
	} else if len(k) < 16 {
		slog.Warn("meilisearch API key is shorter than 16 chars - use a longer secret")
	}

	if (c.VAPID.PublicKey == "") != (c.VAPID.PrivateKey == "") {
		slog.Warn("VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY must both be set or both empty - web push will be disabled")
	}

	if c.OIDC.Enabled && c.OIDC.Issuer != "" && c.OIDC.ClientSecret == "" {
		slog.Warn("OIDC enabled but client secret is empty - set EVEY_OIDC_CLIENT_SECRET")
	}

	switch c.Plugins.SandboxMode {
	case "auto", "landlock", "basic", "bubblewrap", "strict":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("cfg.Plugins.SandboxMode: invalid value %q (expected auto|landlock|basic|bubblewrap|strict)", c.Plugins.SandboxMode))
	}
	if c.Plugins.SandboxMode == "bubblewrap" || c.Plugins.SandboxMode == "strict" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			slog.Warn("cfg.Plugins.SandboxMode requires bwrap but binary not found in $PATH - plugin runner may fail at runtime",
				slog.String("sandbox_mode", c.Plugins.SandboxMode))
		}
	}
	switch c.Plugins.UIDSeparation {
	case "", "auto", "on", "off":
		// valid ("" = unset, treated as the "auto" default by Load / off by the runner)
	default:
		errs = append(errs, fmt.Sprintf("cfg.Plugins.UIDSeparation: invalid value %q (expected auto|on|off)", c.Plugins.UIDSeparation))
	}
	switch c.Plugins.CosignPolicy {
	case "off", "warn", "enforce":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("cfg.Plugins.CosignPolicy: invalid value %q (expected off|warn|enforce)", c.Plugins.CosignPolicy))
	}
	if c.Plugins.CosignPolicy != "off" {
		if c.Plugins.CosignPublicKey == "" {
			if c.Plugins.CosignPolicy == "enforce" {
				// enforce REQUIRES a key: without one the verifier silently
				// degrades to a no-op (see CosignVerifier.VerifySignature) and
				// EVERY plugin install passes unverified while the admin believes
				// signatures are mandatory. That is strictly worse than policy=off
				// (which is at least honest). Fail closed - refuse to start rather
				// than pretend to enforce.
				errs = append(errs, "cfg.Plugins.CosignPublicKey: required when CosignPolicy=enforce (enforce without a public key silently disables verification - set the key or use policy=warn/off)")
			} else {
				// warn is best-effort by design; without a key the verifier is a
				// no-op. Flag it so admins know they're not verifying anything.
				slog.Warn("EVEY_PLUGINS_COSIGN_POLICY is non-off but EVEY_PLUGINS_COSIGN_PUBLIC_KEY is unset - signature verification is disabled. Set the public key to enable, or set policy=off to silence.",
					slog.String("policy", c.Plugins.CosignPolicy))
			}
		} else {
			// Fail-fast on misconfigured cosign key: in `enforce` mode every
			// install would silently fail later; in `warn` we'd flood logs on
			// each upload. Catch unreadable / non-PEM key files at startup.
			if err := validateCosignPublicKey(c.Plugins.CosignPublicKey); err != nil {
				errs = append(errs, fmt.Sprintf("cfg.Plugins.CosignPublicKey: %v", err))
			}
		}
	}
	if n := c.Plugins.CPUNice; n < 0 || n > 19 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.CPUNice: value %d out of range (0-19)", n))
	}
	if m := c.Plugins.MemoryMB; m < 0 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.MemoryMB: value %d must be >= 0 (0 = use default)", m))
	}
	if p := c.Plugins.PIDsMax; p < 0 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.PIDsMax: value %d must be >= 0 (0 = use default)", p))
	}
	if d := c.Plugins.DiskMaxMB; d < 0 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.DiskMaxMB: value %d must be >= 0 (0 = disabled)", d))
	}

	if c.Server.DemoExtended {
		errs = append(errs, validateExtendedDemoAccount(c.Server.DemoUser, c.Server.DemoPassword)...)
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "":
		return true
	}
	return false
}

// defaultSSLMode is the EVEY_DB_SSLMODE applied when the operator sets none.
// TLS to the container's own loopback (the embedded Postgres, or a `docker
// exec evey admin ...` reaching it) protects nothing and the lib/pq driver
// rejects "prefer" outright ("wrong value for sslmode") - so loopback means
// disable. Any other host keeps the historical "prefer": WarnValidate already
// tells the operator to pick require/verify-full there, and every documented
// external layout sets the mode explicitly.
func defaultSSLMode(host string) string {
	if isLoopbackHost(host) {
		return "disable"
	}
	return "prefer"
}

func Load() (*Config, error) {
	dbHost := envStr("EVEY_DB_HOST", "localhost")
	// Read before the literal: a malformed CIDR must stop the boot here, where
	// the variable can be named, rather than deep inside the HTTP wiring.
	trustedProxies, err := trustedProxiesFromEnv("EVEY_TRUSTED_PROXIES")
	if err != nil {
		return nil, err
	}

	baseURL := envStr("EVEY_SERVER_BASE_URL", "http://localhost:9091")

	cfg := &Config{
		Server: ServerConfig{
			Port:               envInt("EVEY_SERVER_PORT", 9090),
			GRPCBindAddr:       envStr("EVEY_GRPC_BIND", "127.0.0.1"),
			BaseURL:            baseURL,
			Name:               envStr("EVEY_SERVER_NAME", DefaultServerName),
			WebDir:             envStr("EVEY_WEB_DIR", "/srv"),
			TrustedProxies:     trustedProxies,
			RealIPHeader:       envStr("EVEY_REAL_IP_HEADER", "X-Forwarded-For"),
			ForwardedProto:     forwardedProtoFromEnv(baseURL),
			LoginRateLimitMax:  envInt("EVEY_RATELIMIT_LOGIN_MAX", 10),
			GlobalRateLimitMax: envInt("EVEY_RATELIMIT_GLOBAL_MAX", 300),
			PluginsEnabled:     envBool("EVEY_PLUGINS_ENABLED", true) && !envBool("EVEY_BENCH_INSTANCE", false),
			BenchInstance:      envBool("EVEY_BENCH_INSTANCE", false),
			Demo:               demoLevel().on,
			DemoExtended:       demoLevel().extended,
			DemoUser:           envStr("EVEY_DEMO_USER", ""),
			DemoPassword:       envStr("EVEY_DEMO_PASSWORD", ""),
			DemoMediaDir:       envStr("EVEY_DEMO_MEDIA_DIR", "/data/demo-media"),
			DemoPluginDir:      envStr("EVEY_DEMO_PLUGIN_DIR", "/data/demo-plugins"),
		},
		DB: DBConfig{
			Host:     dbHost,
			Port:     envInt("EVEY_DB_PORT", 5432),
			User:     envStr("EVEY_DB_USER", "evey"),
			Password: envSecret("EVEY_DB_PASSWORD", "evey"),
			DBName:   envStr("EVEY_DB_NAME", "evey"),
			SSLMode:  envStr("EVEY_DB_SSLMODE", defaultSSLMode(dbHost)),
			// Defaults sized for the weak-NAS target (Celeron, 2-4 GB): 50 open
			// Postgres connections there cost ~250-500 MB of backend RAM for little
			// gain on a low-concurrency media server. 25/10 halves that footprint
			// while staying generous; larger deployments raise it via the env vars.
			MaxOpenConns:   envInt("EVEY_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:   envInt("EVEY_DB_MAX_IDLE_CONNS", 10),
			AppUser:        envStr("EVEY_DB_APP_USER", "evey_app"),
			PrivilegeSplit: envBool("EVEY_DB_PRIVILEGE_SPLIT", true),
		},
		Meilisearch: MeilisearchConfig{
			URL:    envStr("EVEY_MEILI_URL", "http://localhost:7700"),
			APIKey: envSecret("MEILI_MASTER_KEY", "changeme"),
		},
		JWT: JWTConfig{
			PrivateKeyPath:      envStr("EVEY_JWT_PRIVATE_KEY_PATH", "/data/jwt.key"),
			AccessTokenTTL:      envDuration("EVEY_JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL:     envDuration("EVEY_JWT_REFRESH_TTL", 7*24*time.Hour),
			RefreshTokenLongTTL: envDuration("EVEY_JWT_REFRESH_LONG_TTL", 180*24*time.Hour),
		},
		Transcoding: TranscodingConfig{
			MaxConcurrent: envInt("EVEY_TRANSCODING_MAX_CONCURRENT", 2),
			MaxSessions:   envInt("EVEY_TRANSCODING_MAX_SESSIONS", 0),
			// 10 GB, an absolute number on purpose: the cache lives on the
			// docker volume next to whatever else the operator runs, and what
			// the volume is for is their call, not a share the server takes.
			CacheMaxGB:            envInt("EVEY_TRANSCODING_CACHE_MAX_GB", 10),
			CacheDir:              envStr("EVEY_TRANSCODING_CACHE_DIR", "/data/transcode"),
			HWAccelEnabled:        envBool("EVEY_TRANSCODING_HW_ACCEL_ENABLED", false),
			HWBackend:             strings.ToLower(strings.TrimSpace(envStr("EVEY_TRANSCODING_HW_BACKEND", ""))),
			HWMode:                strings.ToLower(strings.TrimSpace(envStr("EVEY_TRANSCODING_HW_MODE", ""))),
			HWDevice:              strings.TrimSpace(envStr("EVEY_TRANSCODING_HW_DEVICE", "")),
			HWFirstSegmentTimeout: envDuration("EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT", 10*time.Second),
			// 0 follows the source bitrate; the web player sizes its forward buffer
			// to the rung so a 70 Mb/s remux stays under the browser's MSE quota.
			MaxVideoBitrateKbps: envInt("EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS", 0),
			// 0 = follow LAN / no remote-specific cap (was 20000: an arbitrary
			// throttle applied to every server that never chose one). 20/40 opt-in.
			MaxVideoBitrateRemoteKbps:    envInt("EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS", 0),
			HWPerVariant:                 envBool("EVEY_TRANSCODING_HW_PER_VARIANT", false),
			SingleVariant:                envBool("EVEY_TRANSCODING_SINGLE_VARIANT", true),
			SoftwareMaxHeightOverride:    envInt("EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT", 0),
			SoftwareHDRMaxHeightOverride: envInt("EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT", 0),
			TrickplayCacheDir:            envStr("EVEY_TRICKPLAY_CACHE_DIR", "/cache/trickplay"),
			AudioRenditionEnabled:        envBool("EVEY_HLS_AUDIO_RENDITION_ENABLED", true),
			CastWasmAudioEnabled:         envBool("EVEY_CAST_WASM_AUDIO_ENABLED", false),
			CopyFallbackRungEnabled:      envBool("EVEY_HLS_COPY_FALLBACK_RUNG", true),
			BenchSDRSamplePath:           envStr("EVEY_BENCH_SDR_SAMPLE", ""),
			BenchSDRSampleURL:            envStr("EVEY_BENCH_SDR_SAMPLE_URL", ""),
			BenchSDRSampleSHA256:         envStr("EVEY_BENCH_SDR_SAMPLE_SHA256", ""),
			BenchHDRSamplePath:           envStr("EVEY_BENCH_HDR_SAMPLE", ""),
			BenchHDRSampleURL:            envStr("EVEY_BENCH_HDR_SAMPLE_URL", ""),
			BenchHDRSampleSHA256:         envStr("EVEY_BENCH_HDR_SAMPLE_SHA256", ""),
		},
		OIDC: OIDCConfig{
			Enabled:             envBool("EVEY_OIDC_ENABLED", false),
			Issuer:              envStr("EVEY_OIDC_ISSUER", ""),
			ClientID:            envStr("EVEY_OIDC_CLIENT_ID", ""),
			ClientSecret:        envStr("EVEY_OIDC_CLIENT_SECRET", ""),
			RedirectURL:         envStr("EVEY_OIDC_REDIRECT_URL", ""),
			AllowedEmailDomains: envStrSlice("EVEY_OIDC_ALLOWED_EMAIL_DOMAINS", nil),
			AllowSignup:         envStr("EVEY_OIDC_ALLOW_SIGNUP", ""),
		},
		Metrics: MetricsConfig{
			RetentionDays:     envInt("EVEY_METRICS_RETENTION_DAYS", 90),
			SampleIntervalSec: envInt("EVEY_METRICS_SAMPLE_INTERVAL_SEC", 60),
			QoERetentionDays:  envInt("EVEY_QOE_RETENTION_DAYS", 30),
			ScrapeToken:       envStr("EVEY_METRICS_SCRAPE_TOKEN", ""),
		},
		Library: LibraryConfig{
			PeriodicScanInterval: envDuration("EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL", 2*time.Minute),
			WatcherEnabled:       envBool("EVEY_LIBRARY_WATCHER_ENABLED", true),
			SeekIndexStateDir:    envStr("EVEY_SEEKINDEX_STATE_DIR", "/data/seek-index"),
		},
		Playback: PlaybackConfig{
			SkipForwardSeconds:   envInt("EVEY_PLAYBACK_SKIP_FORWARD", 30),
			SkipBackwardSeconds:  envInt("EVEY_PLAYBACK_SKIP_BACKWARD", 30),
			SessionRetentionDays: envInt("EVEY_PLAY_SESSION_RETENTION_DAYS", 0),
		},
		Metadata: MetadataConfig{
			Language:              envStr("EVEY_METADATA_LANGUAGE", "en"),
			IntroProvidersEnabled: envBool("EVEY_INTRO_PROVIDERS_ENABLED", false),
			TIDBAPIKey:            envStr("EVEY_INTRO_TIDB_API_KEY", ""),
			SkipPreviewEnabled:    envBool("EVEY_INTRO_SKIP_PREVIEW_ENABLED", false),
			IntroAutoSkipEnabled:  envBool("EVEY_INTRO_AUTO_SKIP_ENABLED", false),
		},
		Backup: BackupConfig{
			Dir:           envStr("EVEY_BACKUP_DIR", "/data/backups"),
			IntervalHours: envInt("EVEY_BACKUP_INTERVAL_HOURS", 0),
			RetentionDays: envInt("EVEY_BACKUP_RETENTION_DAYS", 30),
		},
		Log: LogConfig{
			Level:               envStr("EVEY_LOG_LEVEL", "info"),
			Dir:                 envStr("EVEY_LOG_DIR", "logs"),
			MaxSizeMB:           envInt("EVEY_LOG_MAX_SIZE_MB", 50),
			RetentionDays:       envInt("EVEY_LOG_RETENTION_DAYS", 90),
			PluginRetentionDays: envInt("EVEY_LOG_PLUGIN_RETENTION_DAYS", 10),
			AuditRetentionDays:  envInt("EVEY_AUDIT_LOG_RETENTION_DAYS", 0),
			DevMode:             envBool("EVEY_LOG_DEV_MODE", false),
		},
		RemoteAccess: RemoteAccessConfig{
			Enabled: envBool("EVEY_REMOTE_ACCESS_ENABLED", false),
			// Setting the variable at all IS the choice - including to "false".
			EnabledExplicit:      envSet("EVEY_REMOTE_ACCESS_ENABLED"),
			ExternalURL:          envStr("EVEY_REMOTE_ACCESS_EXTERNAL_URL", ""),
			LANNetworks:          envStrSlice("EVEY_REMOTE_ACCESS_LAN_NETWORKS", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}),
			MaxRemoteBitrateKbps: envInt("EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS", 0),
		},
		SMTP: SMTPConfig{
			Host:     envStr("EVEY_SMTP_HOST", ""),
			Port:     envInt("EVEY_SMTP_PORT", 587),
			Username: envStr("EVEY_SMTP_USERNAME", ""),
			Password: envStr("EVEY_SMTP_PASSWORD", ""),
			From:     envStr("EVEY_SMTP_FROM", ""),
			TLS:      envStr("EVEY_SMTP_TLS", "starttls"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  envStr("VAPID_PUBLIC_KEY", ""),
			PrivateKey: envStr("VAPID_PRIVATE_KEY", ""),
			Subject:    envStr("VAPID_SUBJECT", "mailto:admin@localhost"),
		},
		Tracing: TracingConfig{
			OTLPEndpoint: envStr("EVEY_OTLP_ENDPOINT", ""),
			ServiceName:  envStr("EVEY_TRACING_SERVICE_NAME", "evey"),
			SampleRatio:  envFloat("EVEY_TRACING_SAMPLE_RATIO", 0),
		},
		Plugins: PluginsConfig{
			SandboxMode:     envPluginSandboxMode("EVEY_PLUGINS_SANDBOX_MODE", "auto"),
			MemoryMB:        int64(envInt("EVEY_PLUGINS_MEMORY_MB", 256)),
			DiskMaxMB:       envInt("EVEY_PLUGINS_DISK_MAX_MB", 100),
			CPUNice:         envInt("EVEY_PLUGINS_CPU_NICE", 10),
			PIDsMax:         envInt("EVEY_PLUGINS_PIDS_MAX", 64),
			UIDSeparation:   strings.ToLower(strings.TrimSpace(envStr("EVEY_PLUGINS_UID_SEPARATION", "auto"))),
			CosignPolicy:    strings.ToLower(strings.TrimSpace(envStr("EVEY_PLUGINS_COSIGN_POLICY", "warn"))),
			CosignPublicKey: envStr("EVEY_PLUGINS_COSIGN_PUBLIC_KEY", ""),
		},
	}
	return cfg, nil
}

// envPluginSandboxMode reads the sandbox mode env var and defaults to fallback
// when unset or empty. The value is lowercased for robustness.
func envPluginSandboxMode(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return fallback
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envSecret reads a secret from KEY, or from the file named by KEY_FILE when
// KEY itself is unset - the Docker secrets convention.
//
// The image generates the secrets it can own (the embedded Postgres password,
// the embedded Meilisearch master key) on first boot and persists them under
// /data/keys, so a quick-start install needs no hand-made secret at all. The
// file path is what makes that survive `docker exec`: the entrypoint's exports
// die with the entrypoint, but an ENV baked into the image reaches every
// `evey admin ...` an operator runs in the container. An explicit KEY always
// wins over the file, so an operator who brings their own Postgres or
// Meilisearch keeps configuring them exactly as before. A KEY_FILE that
// cannot be read behaves like an unset secret: the caller's default applies
// and WarnValidate refuses the boot as it does today, with the file named in
// the log rather than silently swallowed.
func envSecret(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	path := os.Getenv(key + "_FILE")
	if path == "" {
		return fallback
	}
	raw, err := os.ReadFile(path) //nolint:gosec // G703 FP: the path comes from the operator's own KEY_FILE environment variable (image default /data/keys/<name>), the same trust as every EVEY_*_PATH the server opens - no request-tainted element
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("config: secret file unreadable - falling back to the default",
				slog.String("key", key+"_FILE"), slog.String("path", path), slog.Any("err", err))
		}
		return fallback
	}
	if v := strings.TrimSpace(string(raw)); v != "" {
		return v
	}
	return fallback
}

// warnBadEnv reports a variable the operator set and we could not parse.
//
// Silence here was indistinguishable from success: `EVEY_HLS_SEGMENT_SECONDS=4s`
// in an int field, or `EVEY_X=tru`, ran the server on the DEFAULT with no trace
// anywhere, while the operator read their own compose file and believed the
// setting applied. The received value is logged as-is: every caller of this
// parses a number, a boolean or a duration, so none of them can carry a secret.
// forwardedProtoFromEnv decides whether the server behaves as served over
// HTTPS (Secure cookies, HSTS, X-Forwarded-Proto trusted from proxies). An
// https:// base URL already says a proxy terminates TLS in front, so the
// scheme is the source of truth; EVEY_FORWARDED_PROTO stays as an explicit
// override for installs that predate the inference.
func forwardedProtoFromEnv(baseURL string) bool {
	fromScheme := strings.HasPrefix(strings.ToLower(baseURL), "https://")
	if os.Getenv("EVEY_FORWARDED_PROTO") == "" {
		return fromScheme
	}
	slog.Warn("config: EVEY_FORWARDED_PROTO is deprecated - the HTTPS behaviour now follows the scheme of EVEY_SERVER_BASE_URL; drop the variable unless you need to force it")
	return envBool("EVEY_FORWARDED_PROTO", fromScheme)
}

func warnBadEnv(key, value, expected string) {
	slog.Warn("config: environment variable ignored, value could not be parsed - falling back to the default",
		slog.String("key", key), slog.String("value", value), slog.String("expected", expected))
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
		warnBadEnv(key, v, "an integer")
	}
	return fallback
}

// envSet reports whether key was provided at all, non-empty - the difference
// between "the operator chose this value" and "this is our default". envBool
// cannot express it: envBool("X", false) returns false both ways.
func envSet(key string) bool {
	return strings.TrimSpace(os.Getenv(key)) != ""
}

// validateExtendedDemoAccount refuses an extended demo that would boot with a
// guessable or untypeable administrator: it is reachable from the internet.
func validateExtendedDemoAccount(user, password string) []string {
	var errs []string
	alnum := func(v string) bool {
		for _, r := range v {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return false
			}
		}
		return v != ""
	}
	if !alnum(user) {
		errs = append(errs, "cfg.Server.DemoUser: EVEY_DEMO_USER is required with EVEY_DEMO=extended, letters and digits only")
	}
	// Any printable ASCII: the operator picks the password, a space or a
	// control character is the only thing a guest cannot be told to type.
	typeable := func(v string) bool {
		for _, r := range v {
			if r <= ' ' || r > '~' {
				return false
			}
		}
		return true
	}
	if len(password) < 12 || !typeable(password) {
		errs = append(errs, "cfg.Server.DemoPassword: EVEY_DEMO_PASSWORD is required with EVEY_DEMO=extended, at least 12 characters, no space")
	}
	return errs
}

type demoSetting struct{ on, extended bool }

// demoLevel reads EVEY_DEMO: a boolean (the public demo) or "extended".
func demoLevel() demoSetting {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("EVEY_DEMO")), "extended") {
		return demoSetting{on: true, extended: true}
	}
	return demoSetting{on: envBool("EVEY_DEMO", false)}
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		warnBadEnv(key, v, "a boolean (1, t, true, 0, f, false)")
	}
	return fallback
}

func envStrSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				result = append(result, s)
			}
		}
		// No conversion can fail here, but a value made only of separators or
		// blanks (",,", " ") yields an EMPTY list rather than the default, and
		// an empty list is a real setting - the operator meant something and
		// got the opposite of what they typed.
		if len(result) == 0 {
			warnBadEnv(key, v, "a comma-separated list with at least one entry (the list is now EMPTY, not the default)")
		}
		return result
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		// The classic miss is a bare number: "30" is not a duration, "30s" is.
		warnBadEnv(key, v, "a duration with a unit (300ms, 30s, 5m, 2h)")
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		warnBadEnv(key, v, "a decimal number")
	}
	return fallback
}
