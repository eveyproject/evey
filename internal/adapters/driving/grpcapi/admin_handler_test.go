package grpcapi

import (
	"context"
	"time"

	appactivity "github.com/eveyproject/evey/internal/app/activity"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	appuser "github.com/eveyproject/evey/internal/app/user"
	activityfakes "github.com/eveyproject/evey/internal/domain/activity/fakes"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

func newTestSystemHandler() *adminSystemHandler {
	events := sharedfakes.NewFakeEventBus()
	roles := userfakes.NewFakeRoleRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	userCmd := appuser.NewCommandService(
		userRepo,
		profileRepo,
		userfakes.NewFakeRefreshTokenRepository(),
		roles,
		userfakes.NewFakeUserRoleRepository(roles),
		userfakes.NewFakePasswordHasher(),
		userfakes.NewFakeTokenGenerator(),
		events,
		7*24*time.Hour,
		userfakes.NewFakeQuickConnectRepository(),
	)
	userQuery := appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)
	activitySvc := appactivity.NewQueryService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		activityfakes.NewFakePlaySessionStatsReader(),
		activityfakes.NewFakePlayHistoryRepository(),
	)
	return &adminSystemHandler{
		userCmd:     userCmd,
		userQuery:   userQuery,
		activitySvc: activitySvc,
		version:     "1.0.0-test",
		// Fixed CPU budget so the auto-capacity assertions (derived from the
		// bench ratio) are deterministic regardless of the host/CI core count -
		// a real cgroup quota of 2 would cap a ratio-3 sample to 2 and flake.
		cpuBudgetFn: func() int { return 8 },
	}
}

func newTestPluginHandler() *adminPluginHandler {
	events := sharedfakes.NewFakeEventBus()
	pluginRepo := pluginfakes.NewFakePluginRepository()
	pluginRuntimeRepo := pluginfakes.NewFakePluginRuntimeRepository()
	pluginRunner := pluginfakes.NewFakePluginRunner()
	pluginParser := pluginfakes.NewFakeManifestParser()
	return &adminPluginHandler{
		pluginInstaller: appplugin.NewInstallerService(pluginRepo, pluginRunner, pluginParser, events),
		pluginRuntime:   appplugin.NewRuntimeService(pluginRepo, pluginRuntimeRepo, pluginRunner, pluginParser, events),
		pluginQuery:     appplugin.NewQueryService(pluginRepo, pluginRuntimeRepo),
		enabled:         true,
	}
}

type fakeLibraryScanner struct {
	scanCalled bool
	libraryID  string
}

func (f *fakeLibraryScanner) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	f.scanCalled = true
	f.libraryID = libraryID
	return 0, 0, nil
}

type fakeAdminLibLister struct {
	libs []*domainlibrary.Library
}

func (f *fakeAdminLibLister) ListLibraries(_ context.Context) ([]*domainlibrary.Library, error) {
	return f.libs, nil
}
