package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appconfig "github.com/evey-project/evey/internal/app/config"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/shared"
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
	Current() domaintranscoding.EncodeCapability
}

// videoSessionActiveReporter is the narrow port GetSystemInfo needs to report
// how many video-encode sessions are live right now. Satisfied by
// *apptranscoding.VideoSessionLimiter.
type videoSessionActiveReporter interface {
	Active() int
}

type adminSystemHandler struct {
	eveyv1.UnimplementedAdminSystemServiceServer
	userCmd        *appuser.CommandService
	userQuery      *appuser.QueryService
	transcodingSvc *apptranscoding.Service
	activitySvc    *appactivity.QueryService
	// hlsManager reports the live HLS session count for the dashboard. Wired to
	// the fan-out selector so the count spans software + hardware producers.
	hlsManager     hlsSessionManager
	libLister      adminLibraryLister
	streamMeter    appactivity.BytesTotaler // optional: nil = no measured bandwidth
	version        string
	startTime      time.Time
	ffmpegPath     string
	translator     *eveyi18n.Translator
	smtpConfigured bool
	serverCfgSvc   *appconfig.Service // optional: nil = server.language/region can't be persisted via setup
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

	// last meter sample, to derive the served rate between dashboard polls.
	meterMu        sync.Mutex
	meterLastTotal int64
	meterLastAt    time.Time
}

// streamingRate derives the measured streaming bits/s since the previous
// dashboard call. Returns -1 on the very first call (nothing to diff against)
// — the frontend shows a dash instead of a misleading 0.
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
		return nil, internalError("count admins", err)
	}
	resp := &eveyv1.SetupStatusResponse{SetupComplete: admins > 0}
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

func (d *adminSystemHandler) GetSystemInfo(ctx context.Context, _ *eveyv1.GetSystemInfoRequest) (*eveyv1.SystemInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetSystemInfo")
	defer span.End()
	admins, adminErr := d.userQuery.CountAdmins(ctx)
	if adminErr != nil {
		slog.Warn("count admins", slog.Any("err", adminErr))
		span.RecordError(adminErr)
	}

	var uptimeSecs int64
	if !d.startTime.IsZero() {
		uptimeSecs = int64(math.Round(time.Since(d.startTime).Seconds()))
	}

	info := &eveyv1.SystemInfo{
		Version:                 d.version,
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
			slog.Warn("detect hardware", slog.Any("err", err))
		} else {
			for _, dev := range devices {
				info.HwDevices = append(info.HwDevices, hwDeviceToProto(dev))
			}
		}
	}

	if d.calibrationSvc != nil {
		capability := d.calibrationSvc.Current()
		auto, evidence := apptranscoding.AutoVideoSessionCapacity(capability.BenchSamples, sysinfo.CPUBudget())
		// A true-nil interface (not a nil-pointer-wrapped one) when the store is
		// absent, so ResolveVideoSessionLimit's own nil check is honest - see the
		// same pattern for pluginCfgRepo in server.go.
		var cfgRepo shared.ServerConfigRepository
		if d.serverCfgSvc != nil {
			cfgRepo = d.serverCfgSvc
		}
		effective := apptranscoding.ResolveVideoSessionLimit(ctx, cfgRepo, auto)
		var active int
		if d.videoLimiter != nil {
			active = d.videoLimiter.Active()
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
		}
	}

	return info, nil
}

func (d *adminSystemHandler) GetServerConfig(ctx context.Context, _ *eveyv1.GetServerConfigRequest) (*eveyv1.ServerConfig, error) {
	_, span := tracing.StartSpan(ctx, "grpc.admin.GetServerConfig")
	defer span.End()
	return &eveyv1.ServerConfig{Values: map[string]string{}}, nil
}

func (d *adminSystemHandler) UpdateServerConfig(ctx context.Context, req *eveyv1.UpdateServerConfigRequest) (*eveyv1.ServerConfig, error) {
	_, span := tracing.StartSpan(ctx, "grpc.admin.UpdateServerConfig")
	defer span.End()
	return &eveyv1.ServerConfig{Values: req.Values}, nil
}

func (d *adminSystemHandler) SetupWizard(ctx context.Context, req *eveyv1.SetupWizardRequest) (*eveyv1.SetupWizardResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.SetupWizard")
	defer span.End()
	// IMPORTANT: create the admin FIRST — if setup is already done Setup()
	// returns ErrSetupDone and we must not have mutated server.language/region
	// in the meantime. This endpoint is public (pre-setup) so any side-effect
	// before the Setup check is an unauthenticated write on a running server.
	// The chosen language/region are passed through as setup options so the
	// admin's default profile is created with the picked locale — otherwise
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
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("setup", err)
	}
	// Persist language/region AFTER Setup() confirmed this is a fresh install.
	// server.language also drives metadata provider + future profile defaults.
	if d.serverCfgSvc != nil {
		if req.ServerLanguage != "" {
			if err := d.serverCfgSvc.Set(ctx, "server.language", req.ServerLanguage); err != nil {
				slog.Warn("setup: persist server.language", slog.Any("err", err))
			}
		}
		if req.ServerRegion != "" {
			if err := d.serverCfgSvc.Set(ctx, "server.region", req.ServerRegion); err != nil {
				slog.Warn("setup: persist server.region", slog.Any("err", err))
			}
		}
	}
	access, refresh, err := d.userCmd.Login(ctx, req.AdminUsername, req.AdminPassword)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("login after setup", err)
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
		return nil, internalError("get server stats", err)
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
	// Best effort — the dashboard stays useful without it.
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
	slog.Warn("admin-requested server restart", slog.String("requested_by", who))
	// Delay so the RPC response reaches the client before the drain starts.
	time.AfterFunc(restartGraceDelay, d.restartFn)
	return &eveyv1.RestartServerResponse{
		Restarting: true,
		Message:    "Server is restarting; it will be back shortly.",
	}, nil
}
