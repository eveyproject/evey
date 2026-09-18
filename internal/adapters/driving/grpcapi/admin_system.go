package grpcapi

import (
	"context"
	"errors"
	apploadtest "github.com/evey-project/evey/internal/app/loadtest"
	"github.com/evey-project/evey/internal/domain/user"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appconfig "github.com/evey-project/evey/internal/app/config"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/config"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// calibrationSnapshotProvider is the narrow port GetSystemInfo needs to derive
// the auto video-session capacity from the boot benchmark's evidence
// (EncodeCapability.BenchSamples), without depending on the whole
// CalibrationService type. Satisfied by *apptranscoding.CalibrationService.
type calibrationSnapshotProvider interface {
	Current() domaintranscoding.HostCapabilitySnapshot
	// IsBenchmarking gates the load test: a calibration bench and a load test
	// running together would measure each other's contention, not the host.
	IsBenchmarking() bool
}

// videoSessionActiveReporter is the narrow port GetSystemInfo needs to report
// video-encode session usage: the plain per-stream counter (Active()) and,
// when a cost model is wired to the limiter, the budget view (Used/Budget/
// Rungs). Satisfied by *apptranscoding.VideoSessionLimiter - Used/Budget/
// Rungs all report the zero value (0 / 0 / empty) when no cost model is
// wired, so the admin panel harmlessly falls back to the counter-only view.
type videoSessionActiveReporter interface {
	Active() int
	Used() float64
	Budget() float64
	Rungs() []apptranscoding.RungFit
	// UsedFor/BudgetFor report a single pool's slice of the aggregate
	// Used()/Budget() above - the CPU (software ladders) and GPU (hw ladder)
	// budgets are independent, so the aggregate view is misleading once both
	// are in play (see VideoSessionCapacity.pools). Classes/RungsFor let the
	// panel discover which ladder classes have evidence and their per-rung
	// cost, to build the per-pool breakdown.
	UsedFor(pool apptranscoding.ResourcePool) float64
	BudgetFor(pool apptranscoding.ResourcePool) float64
	RungsFor(class apptranscoding.LadderClass) []apptranscoding.RungFit
	Classes() []apptranscoding.LadderClass
	// AutoCountCap is the count cap the calibration derives, and Limit the one
	// actually enforced (the former with the admin's reduction applied). Both
	// come FROM the limiter so the panel can never disagree with admission.
	AutoCountCap() int
	Limit() int
}

type adminSystemHandler struct {
	eveyv1.UnimplementedAdminSystemServiceServer
	userCmd        *appuser.CommandService
	userQuery      *appuser.QueryService
	transcodingSvc *apptranscoding.Service
	activitySvc    *appactivity.QueryService
	// hlsManager reports the live HLS session count for the dashboard. Wired to
	// the fan-out selector so the count spans software + hardware producers.
	hlsManager  hlsSessionManager
	libLister   adminLibraryLister
	streamMeter appactivity.BytesTotaler // optional: nil = no measured bandwidth
	version     string
	// commit / buildDate identify the exact code this process runs. Blank means
	// the wiring never stamped them, and the handler reports "unknown" rather
	// than an empty string - see buildStampOrUnknown.
	commit         string
	buildDate      string
	startTime      time.Time
	ffmpegPath     string
	translator     *eveyi18n.Translator
	smtpConfigured bool
	serverCfgSvc   *appconfig.Service // optional: nil = server.language/region can't be persisted via setup
	// cfg is the live runtime config: the server name is read from it (the
	// admin PUT updates it in place) and the setup wizard writes it there so
	// the name applies without a restart. Optional: nil reports the default.
	cfg *config.Config
	// restartFn triggers a graceful shutdown so a supervisor restarts the
	// process. nil disables the restart RPC (unsupervised / not wired).
	restartFn func()
	// calibrationSvc supplies the boot benchmark's evidence for the
	// video-session auto-capacity block on GetSystemInfo. Optional: nil skips
	// populating SystemInfo.VideoSessions entirely.
	calibrationSvc calibrationSnapshotProvider
	// videoLimiter reports live video-encode session usage (Active()) for the
	// same block, and is the SAME *apptranscoding.VideoSessionLimiter instance
	// shared between the software and hardware producers. Optional: nil reports
	// active=0.
	videoLimiter videoSessionActiveReporter
	// cpuBudgetFn reports the host's usable core count for the auto-capacity
	// calc. nil = sysinfo.CPUBudget (the real cgroup quota); tests inject a
	// fixed value so the auto-capacity is deterministic regardless of the CI
	// runner's core count.
	cpuBudgetFn func() int

	// loadTest runs the built-in capacity measurement (nil = RPC answers
	// Unimplemented). loadTestBaseURL is the server's own loopback origin the
	// synthetic viewers hit.
	loadTest        loadTestRunner
	loadTestBaseURL string
	// loadTestPublicURL is the reverse-proxy origin real viewers come through.
	// Preferred when reachable; the loopback is the fallback.
	loadTestPublicURL string
	// loadTestToken mints the credential the synthetic viewers authenticate
	// with. It must OUTLIVE the run: a load test lasts many minutes, and the
	// caller's own browser token expires long before that - borrowing it means
	// the run dies on a 401 mid-ramp and reports an expiry as if it were a
	// capacity verdict. nil falls back to the caller's bearer.
	loadTestToken loadTestTokenMinter

	// last meter sample, to derive the served rate between dashboard polls.
	meterMu        sync.Mutex
	meterLastTotal int64
	meterLastAt    time.Time
}

// loadTestRunner is the narrow port the three load-test RPCs need.
type loadTestRunner interface {
	Start(ctx context.Context, cfg apploadtest.Config) error
	Status() apploadtest.Status
	Cancel()
}

// loadTestTokenMinter forges a credential for the synthetic viewers that lives
// as long as the run does. Same claims as the admin who launched it (same user,
// same permissions, same epoch - revoking the session still kills it); only the
// lifetime differs.
type loadTestTokenMinter interface {
	GenerateAccessTokenWithTTL(claims user.Claims, ttl time.Duration) (string, error)
}

// streamingRate derives the measured streaming bits/s since the previous
// dashboard call. Returns -1 on the very first call (nothing to diff against)
// - the frontend shows a dash instead of a misleading 0.
func (d *adminSystemHandler) streamingRate(total int64) int64 {
	d.meterMu.Lock()
	defer d.meterMu.Unlock()
	now := time.Now()
	defer func() { d.meterLastTotal, d.meterLastAt = total, now }()
	if d.meterLastAt.IsZero() {
		return -1
	}
	elapsed := now.Sub(d.meterLastAt).Seconds()
	if elapsed <= 0 || total < d.meterLastTotal {
		return -1
	}
	return int64(float64(total-d.meterLastTotal) * 8 / elapsed)
}

func (d *adminSystemHandler) GetSetupStatus(ctx context.Context, _ *eveyv1.GetSetupStatusRequest) (*eveyv1.SetupStatusResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetSetupStatus")
	defer span.End()
	admins, err := d.userQuery.CountAdmins(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "count admins", err)
	}
	resp := &eveyv1.SetupStatusResponse{SetupComplete: admins > 0, ServerName: d.serverName()}
	// Public projection of the recovery flag so the logged-out login page can
	// show the "Forgot password?" link (mirrors GetSystemInfo). It only reads
	// config (SMTP configured + auth.password_recovery_enabled), no auth needed.
	if d.userCmd != nil {
		resp.PasswordRecoveryEnabled = d.userCmd.IsPasswordRecoveryEnabled(ctx)
	}
	if d.serverCfgSvc != nil {
		if e, err := d.serverCfgSvc.Get(ctx, "server.language"); err == nil {
			resp.ServerLanguage = e.Value
		}
		if e, err := d.serverCfgSvc.Get(ctx, "server.region"); err == nil {
			resp.ServerRegion = e.Value
		}
	}
	return resp, nil
}

// serverName is the instance name the pre-auth surfaces show. Read from the
// live config so an admin rename applies without a restart.
func (d *adminSystemHandler) serverName() string {
	if d.cfg == nil {
		return config.DefaultServerName
	}
	return d.cfg.EffectiveServerName()
}

// applyServerName persists the wizard's server name under the SAME key the
// admin settings PUT uses (server.name) and applies it to the live config, so
// the login page and the mails carry it from the first boot on and the
// settings card shows it as the override it is.
func (d *adminSystemHandler) applyServerName(ctx context.Context, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if d.serverCfgSvc != nil {
		if err := d.serverCfgSvc.Set(ctx, "server.name", name); err != nil {
			slog.WarnContext(ctx, "setup: persist server.name", slog.Any("err", err))
		}
	}
	if d.cfg != nil {
		d.cfg.ApplyServerSettings(func() { d.cfg.Server.Name = name })
	}
}

// unknownBuildStamp mirrors cmd/evey's unknownStamp and the HTTP handler's:
// the value reported when a build cannot name itself.
const unknownBuildStamp = "unknown"

// buildStampOrUnknown keeps a blank build stamp off the wire: "" is
// indistinguishable from an older server that never had the field, and the
// caller would have to guess. "unknown" is an explicit answer, and
// scripts/verify-deploy.sh fails on it instead of assuming a match.
func buildStampOrUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return unknownBuildStamp
	}
	return s
}

func (d *adminSystemHandler) GetSystemInfo(ctx context.Context, _ *eveyv1.GetSystemInfoRequest) (*eveyv1.SystemInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetSystemInfo")
	defer span.End()
	admins, adminErr := d.userQuery.CountAdmins(ctx)
	if adminErr != nil {
		slog.WarnContext(ctx, "count admins", slog.Any("err", adminErr))
		span.RecordError(adminErr)
	}

	var uptimeSecs int64
	if !d.startTime.IsZero() {
		uptimeSecs = int64(math.Round(time.Since(d.startTime).Seconds()))
	}

	info := &eveyv1.SystemInfo{
		Version:                 d.version,
		Commit:                  buildStampOrUnknown(d.commit),
		BuildDate:               buildStampOrUnknown(d.buildDate),
		Os:                      runtime.GOOS,
		Arch:                    runtime.GOARCH,
		GoVersion:               runtime.Version(),
		SetupComplete:           admins > 0,
		UptimeSeconds:           uptimeSecs,
		FfmpegVersion:           sysinfo.FFmpegVersion(d.ffmpegPath),
		PasswordRecoveryEnabled: d.userCmd.IsPasswordRecoveryEnabled(ctx),
		SmtpConfigured:          d.smtpConfigured,
	}

	if d.transcodingSvc != nil {
		devices, err := d.transcodingSvc.DetectHardware(ctx)
		if err != nil {
			slog.WarnContext(ctx, "detect hardware", slog.Any("err", err))
		} else {
			for _, dev := range devices {
				info.HwDevices = append(info.HwDevices, hwDeviceToProto(dev))
			}
		}
	}

	if d.calibrationSvc != nil {
		capability := d.calibrationSvc.Current()
		cpuBudget := sysinfo.CPUBudget()
		if d.cpuBudgetFn != nil {
			cpuBudget = d.cpuBudgetFn()
		}
		// evidence is the "why" narrative only - the tallest passing software rung.
		// It is NOT the capacity: admission is governed by the per-pool cost
		// budgets, and deriving a second number here is what used to make the
		// panel report an effective limit nobody enforced.
		_, evidence := apptranscoding.AutoVideoSessionCapacity(capability.BenchSamples, cpuBudget)
		var active, auto, effective int
		var budgetUsed, budgetTotal float64
		var rungFits []*eveyv1.VideoRungFit
		var pools []*eveyv1.VideoPool
		if d.videoLimiter != nil {
			active = d.videoLimiter.Active()
			auto = d.videoLimiter.AutoCountCap()
			// The limit ACTUALLY enforced, read from the limiter itself rather
			// than re-derived: the admin override was already applied when it was
			// armed (ResolveVideoSessionLimit, at the wiring site).
			effective = d.videoLimiter.Limit()
			budgetUsed = d.videoLimiter.Used()
			budgetTotal = d.videoLimiter.Budget()
			for _, r := range d.videoLimiter.Rungs() {
				rungFits = append(rungFits, videoRungFitToProto(r))
			}
			pools = d.videoSessionPools()
		}
		info.VideoSessions = &eveyv1.VideoSessionCapacity{
			AutoCapacity:   int32(auto),
			EffectiveLimit: int32(effective),
			Active:         int32(active),
			Evidence: &eveyv1.VideoCapacityEvidence{
				Height:      int32(evidence.Height),
				Fps:         evidence.FPS,
				RequiredFps: evidence.RequiredFPS,
				Ratio:       evidence.Ratio,
				CpuBudget:   int32(evidence.CPUBudget),
				Derived:     evidence.Derived,
			},
			BudgetUsed:  budgetUsed,
			BudgetTotal: budgetTotal,
			RungFits:    rungFits,
			Pools:       pools,
			// The decode half of the calibration - what makes the rung costs
			// above readable instead of a number without its condition.
			BenchSource:      sourceClassToProto(capability.BenchSource),
			DecodeThroughput: decodeThroughputToProto(capability.DecodeSamples),
		}
	}

	return info, nil
}

// videoRungFitToProto converts one calibrated rung's cost/max-streams to its
// proto projection.
func videoRungFitToProto(r apptranscoding.RungFit) *eveyv1.VideoRungFit {
	out := &eveyv1.VideoRungFit{
		Height:     int32(r.Height),
		Cost:       r.Cost,
		MaxStreams: int32(r.MaxStreams),
	}
	for _, f := range r.BySource {
		out.BySource = append(out.BySource, &eveyv1.VideoSourceFit{
			Source:     sourceClassToProto(f.Source),
			Cost:       f.Cost,
			MaxStreams: int32(f.MaxStreams),
		})
	}
	return out
}

func sourceClassToProto(c domaintranscoding.SourceClass) *eveyv1.SourceClass {
	return &eveyv1.SourceClass{Codec: c.Codec, Height: int32(c.Height)}
}

// videoSessionPools builds the per-resource-pool breakdown (see
// VideoSessionCapacity.pools): a "cpu" pool for every software ladder class
// with calibration evidence, and a "gpu" pool ONLY when the host also has a
// LadderClassHW table (d.videoLimiter.Classes() reports it) - a software-only
// box never gets a hollow gpu row. nil when the limiter has no cost model
// wired at all (Classes() then reports none).
func (d *adminSystemHandler) videoSessionPools() []*eveyv1.VideoPool {
	classes := d.videoLimiter.Classes()
	if len(classes) == 0 {
		return nil
	}

	var cpuClasses, gpuClasses []*eveyv1.VideoClassRungs
	for _, class := range classes {
		rungs := &eveyv1.VideoClassRungs{LadderClass: string(class)}
		for _, r := range d.videoLimiter.RungsFor(class) {
			rungs.RungFits = append(rungs.RungFits, videoRungFitToProto(r))
		}
		if class == apptranscoding.LadderClassHW {
			gpuClasses = append(gpuClasses, rungs)
		} else {
			cpuClasses = append(cpuClasses, rungs)
		}
	}

	var pools []*eveyv1.VideoPool
	if len(cpuClasses) > 0 {
		pools = append(pools, &eveyv1.VideoPool{
			Pool:        string(apptranscoding.PoolCPU),
			BudgetUsed:  d.videoLimiter.UsedFor(apptranscoding.PoolCPU),
			BudgetTotal: d.videoLimiter.BudgetFor(apptranscoding.PoolCPU),
			Classes:     cpuClasses,
		})
	}
	if len(gpuClasses) > 0 {
		pools = append(pools, &eveyv1.VideoPool{
			Pool:        string(apptranscoding.PoolGPU),
			BudgetUsed:  d.videoLimiter.UsedFor(apptranscoding.PoolGPU),
			BudgetTotal: d.videoLimiter.BudgetFor(apptranscoding.PoolGPU),
			Classes:     gpuClasses,
		})
	}
	return pools
}

// GetServerConfig / UpdateServerConfig are the map-shaped view of the SAME
// server_config store ServerConfigService serves per key. Both used to be
// stubs: Get returned an empty map and Update echoed the request back without
// writing anything, so a caller got a 200 and believed a setting had been
// saved. The mobile admin panel is wired here (the web uses the per-key
// service), so its "Basic" section reported success and persisted nothing.
//
// A missing serverCfgSvc now returns Unimplemented rather than a fake success:
// an endpoint that lies is worse than one that admits it cannot serve.
func (d *adminSystemHandler) GetServerConfig(ctx context.Context, _ *eveyv1.GetServerConfigRequest) (*eveyv1.ServerConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetServerConfig")
	defer span.End()
	if d.serverCfgSvc == nil {
		return nil, status.Error(codes.Unimplemented, "server config service not wired")
	}
	entries, err := d.serverCfgSvc.List(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, internalError(ctx, "get server config", err)
	}
	values := make(map[string]string, len(entries))
	for _, e := range entries {
		values[e.Key] = e.Value
	}
	return &eveyv1.ServerConfig{Values: values}, nil
}

func (d *adminSystemHandler) UpdateServerConfig(ctx context.Context, req *eveyv1.UpdateServerConfigRequest) (*eveyv1.ServerConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UpdateServerConfig")
	defer span.End()
	if d.serverCfgSvc == nil {
		return nil, status.Error(codes.Unimplemented, "server config service not wired")
	}
	for k, v := range req.Values {
		if k == "" {
			return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.config.key_required")
		}
		if err := d.serverCfgSvc.Set(ctx, k, v); err != nil {
			span.RecordError(err)
			return nil, internalError(ctx, "update server config", err)
		}
	}
	// Read back rather than echo: the response now describes what the store
	// actually holds, which is exactly the confusion the echo created.
	return d.GetServerConfig(ctx, &eveyv1.GetServerConfigRequest{})
}

func (d *adminSystemHandler) SetupWizard(ctx context.Context, req *eveyv1.SetupWizardRequest) (*eveyv1.SetupWizardResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.SetupWizard")
	defer span.End()
	// IMPORTANT: create the admin FIRST - if setup is already done Setup()
	// returns ErrSetupDone and we must not have mutated server.language/region
	// in the meantime. This endpoint is public (pre-setup) so any side-effect
	// before the Setup check is an unauthenticated write on a running server.
	// The chosen language/region are passed through as setup options so the
	// admin's default profile is created with the picked locale - otherwise
	// the profile falls back to the DB column default ("en") and the JWT
	// locale claim issued by the Login below would downgrade the UI back to
	// English on the very next session.
	u, err := d.userCmd.Setup(ctx, req.AdminUsername, req.AdminPassword, req.AdminEmail,
		appuser.WithSetupLocale(req.ServerLanguage),
		appuser.WithSetupRegion(req.ServerRegion),
	)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, appuser.ErrSetupDone) {
			span.SetStatus(otelcodes.Error, "setup already completed")
			return nil, d.translator.TranslatedError(ctx, codes.AlreadyExists, "errors.admin.setup_already_completed")
		}
		// Bad wizard input (empty username, too-short password) is a client
		// error, not a server fault: map it to InvalidArgument with a clear
		// reason instead of a generic 500 logged at ERROR, so the wizard can
		// surface the actual problem.
		if errors.Is(err, appuser.ErrUsernameRequired) {
			span.SetStatus(otelcodes.Error, "username required")
			return nil, status.Error(codes.InvalidArgument, "username is required")
		}
		if errors.Is(err, appuser.ErrPasswordTooShort) {
			span.SetStatus(otelcodes.Error, "password too short")
			return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.password_too_short")
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "setup", err)
	}
	// Persist language/region AFTER Setup() confirmed this is a fresh install.
	// server.language also drives metadata provider + future profile defaults.
	if d.serverCfgSvc != nil {
		if req.ServerLanguage != "" {
			if err := d.serverCfgSvc.Set(ctx, "server.language", req.ServerLanguage); err != nil {
				slog.WarnContext(ctx, "setup: persist server.language", slog.Any("err", err))
			}
		}
		if req.ServerRegion != "" {
			if err := d.serverCfgSvc.Set(ctx, "server.region", req.ServerRegion); err != nil {
				slog.WarnContext(ctx, "setup: persist server.region", slog.Any("err", err))
			}
		}
	}
	d.applyServerName(ctx, req.ServerName)
	access, refresh, err := d.userCmd.Login(ctx, req.AdminUsername, req.AdminPassword)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "login after setup", err)
	}
	return &eveyv1.SetupWizardResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ServerId:     string(u.ID),
	}, nil
}

func (d *adminSystemHandler) GetDashboard(ctx context.Context, _ *eveyv1.GetDashboardRequest) (*eveyv1.DashboardInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetDashboard")
	defer span.End()
	stats, err := d.activitySvc.GetServerStats(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get server stats", err)
	}

	sys := sysinfo.Collect()

	activeTranscodes := int32(stats.ActiveTranscodes)
	if d.hlsManager != nil {
		hlsCount := d.hlsManager.ActiveSessionCount()
		if int32(hlsCount) > activeTranscodes {
			activeTranscodes = int32(hlsCount)
		}
	}

	resp := &eveyv1.DashboardInfo{
		ActiveStreams:    int32(stats.ActiveStreams),
		ActiveTranscodes: activeTranscodes,
		TotalMediaItems:  int32(stats.TotalItems),
		TotalUsers:       int32(stats.TotalUsers),
		CpuPercent:       sys.CPUPercent,
		MemoryPercent:    sys.MemoryPercent,
		StreamingBps:     -1,
	}

	if d.streamMeter != nil {
		total := d.streamMeter.Total()
		resp.StreamingBytesTotal = total
		resp.StreamingBps = d.streamingRate(total)
	}
	// Theoretical outgoing rate: summed source bitrate of active sessions.
	// Best effort - the dashboard stays useful without it.
	if bps, err := d.activitySvc.GetActiveSessionsBitrate(ctx); err == nil {
		resp.ActiveSessionsBps = bps
	}

	if d.libLister != nil {
		libs, err := d.libLister.ListLibraries(ctx)
		if err == nil {
			var paths []string
			for _, lib := range libs {
				paths = append(paths, lib.Paths...)
			}
			for _, st := range sysinfo.GetStorageStats(paths) {
				resp.Storage = append(resp.Storage, &eveyv1.StorageInfo{
					Path:       st.Path,
					Paths:      st.Paths,
					TotalBytes: st.TotalBytes,
					UsedBytes:  st.UsedBytes,
					FreeBytes:  st.FreeBytes,
				})
			}
		}
	}

	return resp, nil
}

// restartGraceDelay is how long RestartServer waits before triggering the drain,
// so the RPC response flushes to the client first. A var so tests can shrink it.
var restartGraceDelay = 700 * time.Millisecond

// RestartServer triggers a graceful shutdown after briefly delaying so this
// response can flush; a process supervisor (Docker `restart: unless-stopped`,
// systemd) then brings the server back up. Returns Unavailable when no restart
// hook is wired (the process is not supervised, so exiting would just stop it).
func (d *adminSystemHandler) RestartServer(ctx context.Context, _ *eveyv1.RestartServerRequest) (*eveyv1.RestartServerResponse, error) {
	if d.restartFn == nil {
		return nil, status.Error(codes.Unavailable, "server restart is not available in this deployment")
	}
	who := "admin"
	if c := interceptors.ClaimsFromContext(ctx); c != nil && c.Username != "" {
		who = c.Username
	}
	slog.WarnContext(ctx, "admin-requested server restart", slog.String("requested_by", who))
	// Delay so the RPC response reaches the client before the drain starts.
	time.AfterFunc(restartGraceDelay, d.restartFn)
	return &eveyv1.RestartServerResponse{
		Restarting: true,
		Message:    "Server is restarting; it will be back shortly.",
	}, nil
}

// decodeThroughputToProto projects the measured decode-only rates.
func decodeThroughputToProto(samples []domaintranscoding.DecodeRungSample) []*eveyv1.DecodeThroughput {
	out := make([]*eveyv1.DecodeThroughput, 0, len(samples))
	for _, s := range samples {
		out = append(out, &eveyv1.DecodeThroughput{
			Codec: s.Codec, Height: int32(s.Height), Fps: s.FPS,
		})
	}
	return out
}
