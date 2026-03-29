package grpcapi

import (
	"context"
	"time"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	pluginfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

func newTestAdminHandler() *adminHandler {
	systemDel := newTestSystemDelegate()
	pluginDel := newTestPluginDelegate()
	return &adminHandler{
		system:    systemDel,
		plugin:    pluginDel,
		scheduler: &adminSchedulerDelegate{},
		media:     &adminMediaDelegate{},
		audit: &adminAuditDelegate{
			activitySvc: systemDel.activitySvc,
		},
	}
}

func newTestSystemDelegate() *adminSystemDelegate {
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
	return &adminSystemDelegate{
		userCmd:     userCmd,
		userQuery:   userQuery,
		activitySvc: activitySvc,
		version:     "1.0.0-test",
	}
}

func newTestPluginDelegate() *adminPluginDelegate {
	events := sharedfakes.NewFakeEventBus()
	pluginRepo := pluginfakes.NewFakePluginRepository()
	pluginRuntimeRepo := pluginfakes.NewFakePluginRuntimeRepository()
	pluginRunner := pluginfakes.NewFakePluginRunner()
	pluginParser := pluginfakes.NewFakeManifestParser()
	return &adminPluginDelegate{
		pluginInstaller: appplugin.NewInstallerService(pluginRepo, pluginRunner, pluginParser, events),
		pluginRuntime:   appplugin.NewRuntimeService(pluginRepo, pluginRuntimeRepo, pluginRunner, pluginParser, events),
		pluginQuery:     appplugin.NewQueryService(pluginRepo, pluginRuntimeRepo),
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
