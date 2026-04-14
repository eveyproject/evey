package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"runtime"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
	"google.golang.org/grpc/codes"
)

type adminSystemHandler struct {
	eveyv1.UnimplementedAdminSystemServiceServer
	userCmd        *appuser.CommandService
	userQuery      *appuser.QueryService
	transcodingSvc *apptranscoding.Service
	activitySvc    *appactivity.QueryService
	hlsManager     *apptranscoding.HLSManager
	libLister      adminLibraryLister
	version        string
	startTime      time.Time
	ffmpegPath     string
	translator     *eveyi18n.Translator
	smtpConfigured bool
}

func (d *adminSystemHandler) GetSetupStatus(ctx context.Context, _ *eveyv1.GetSetupStatusRequest) (*eveyv1.SetupStatusResponse, error) {
	admins, err := d.userQuery.CountAdmins(ctx)
	if err != nil {
		return nil, internalError("count admins", err)
	}
	return &eveyv1.SetupStatusResponse{SetupComplete: admins > 0}, nil
}

func (d *adminSystemHandler) GetSystemInfo(ctx context.Context, _ *eveyv1.GetSystemInfoRequest) (*eveyv1.SystemInfo, error) {
	admins, adminErr := d.userQuery.CountAdmins(ctx)
	if adminErr != nil {
		slog.Warn("count admins", slog.Any("err", adminErr))
	}

	var uptimeSecs int64
	if !d.startTime.IsZero() {
		uptimeSecs = int64(math.Round(time.Since(d.startTime).Seconds()))
	}

	info := &eveyv1.SystemInfo{
		Version:                  d.version,
		Os:                       runtime.GOOS,
		Arch:                     runtime.GOARCH,
		GoVersion:                runtime.Version(),
		SetupComplete:            admins > 0,
		UptimeSeconds:            uptimeSecs,
		FfmpegVersion:            sysinfo.FFmpegVersion(d.ffmpegPath),
		PasswordRecoveryEnabled:  d.userCmd.IsPasswordRecoveryEnabled(ctx),
		SmtpConfigured:           d.smtpConfigured,
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

	return info, nil
}

func (d *adminSystemHandler) GetServerConfig(_ context.Context, _ *eveyv1.GetServerConfigRequest) (*eveyv1.ServerConfig, error) {
	return &eveyv1.ServerConfig{Values: map[string]string{}}, nil
}

func (d *adminSystemHandler) UpdateServerConfig(_ context.Context, req *eveyv1.UpdateServerConfigRequest) (*eveyv1.ServerConfig, error) {
	return &eveyv1.ServerConfig{Values: req.Values}, nil
}

func (d *adminSystemHandler) SetupWizard(ctx context.Context, req *eveyv1.SetupWizardRequest) (*eveyv1.SetupWizardResponse, error) {
	u, err := d.userCmd.Setup(ctx, req.AdminUsername, req.AdminPassword, req.AdminEmail)
	if err != nil {
		if errors.Is(err, appuser.ErrSetupDone) {
			return nil, d.translator.TranslatedError(ctx, codes.AlreadyExists, "errors.admin.setup_already_completed")
		}
		return nil, internalError("setup", err)
	}
	access, refresh, err := d.userCmd.Login(ctx, req.AdminUsername, req.AdminPassword)
	if err != nil {
		return nil, internalError("login after setup", err)
	}
	return &eveyv1.SetupWizardResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ServerId:     string(u.ID),
	}, nil
}

func (d *adminSystemHandler) GetDashboard(ctx context.Context, _ *eveyv1.GetDashboardRequest) (*eveyv1.DashboardInfo, error) {
	stats, err := d.activitySvc.GetServerStats(ctx)
	if err != nil {
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
		ActiveStreams:     int32(stats.ActiveStreams),
		ActiveTranscodes:  activeTranscodes,
		TotalMediaItems:   int32(stats.TotalItems),
		TotalUsers:        int32(stats.TotalUsers),
		CpuPercent:        sys.CPUPercent,
		MemoryPercent:     sys.MemoryPercent,
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
					TotalBytes: st.TotalBytes,
					UsedBytes:  st.UsedBytes,
					FreeBytes:  st.FreeBytes,
				})
			}
		}
	}

	return resp, nil
}
