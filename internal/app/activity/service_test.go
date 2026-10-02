package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	"github.com/evey-project/evey/internal/domain/activity"
	actfakes "github.com/evey-project/evey/internal/domain/activity/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServices() (
	*appactivity.CommandService,
	*appactivity.QueryService,
	*actfakes.FakeLoginHistoryRepository,
	*actfakes.FakeServerMetricRepository,
	*actfakes.FakeAuditLogRepository,
	*actfakes.FakePlaySessionStatsReader,
	*actfakes.FakePlayHistoryRepository,
) {
	loginRepo := actfakes.NewFakeLoginHistoryRepository()
	metricRepo := actfakes.NewFakeServerMetricRepository()
	auditRepo := actfakes.NewFakeAuditLogRepository()
	statsReader := actfakes.NewFakePlaySessionStatsReader()
	playHistoryRepo := actfakes.NewFakePlayHistoryRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appactivity.NewCommandService(loginRepo, metricRepo, auditRepo, bus)
	querySvc := appactivity.NewQueryService(loginRepo, metricRepo, auditRepo, statsReader, playHistoryRepo)
	return cmdSvc, querySvc, loginRepo, metricRepo, auditRepo, statsReader, playHistoryRepo
}

func TestRecordLogin(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, _, _, _ := newTestServices()

	err := cmdSvc.RecordLogin(context.Background(), &activity.LoginEntry{
		ID:         "l1",
		UserID:     "u1",
		ClientIP:   "127.0.0.1",
		AuthMethod: "local",
		Success:    true,
	})
	require.NoError(t, err)

	history, err := querySvc.GetLoginHistory(context.Background(), "u1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, history, 1)
}

func TestRecordLogin_SetsCreatedAt(t *testing.T) {
	t.Parallel()
	cmdSvc, _, loginRepo, _, _, _, _ := newTestServices()

	before := time.Now()
	entry := &activity.LoginEntry{ID: "l2", UserID: "u1", AuthMethod: "local", Success: true}
	require.NoError(t, cmdSvc.RecordLogin(context.Background(), entry))
	after := time.Now()

	stored, err := loginRepo.GetByUserID(context.Background(), "u1", 10, 0)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.True(t, stored[0].CreatedAt.After(before) || stored[0].CreatedAt.Equal(before))
	assert.True(t, stored[0].CreatedAt.Before(after) || stored[0].CreatedAt.Equal(after))
}

func TestRecordLogin_Error(t *testing.T) {
	t.Parallel()
	cmdSvc, _, loginRepo, _, _, _, _ := newTestServices()
	loginRepo.Err = errors.New("db error")

	err := cmdSvc.RecordLogin(context.Background(), &activity.LoginEntry{ID: "l1", UserID: "u1"})
	require.Error(t, err)
}

func TestRecordMetric(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, _, _, _ := newTestServices()

	now := time.Now()
	err := cmdSvc.RecordMetric(context.Background(), &activity.ServerMetric{
		ID:         "m1",
		CPUPercent: 42.5,
	})
	require.NoError(t, err)

	metrics, err := querySvc.GetServerMetrics(context.Background(), now.Add(-time.Second), now.Add(time.Second))
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.InDelta(t, 42.5, metrics[0].CPUPercent, 0.01)
}

func TestGetServerMetrics_TimeRange(t *testing.T) {
	t.Parallel()
	_, querySvc, _, metricRepo, _, _, _ := newTestServices()

	base := time.Now()
	_ = metricRepo.Create(context.Background(), &activity.ServerMetric{ID: "m1", CPUPercent: 10, RecordedAt: base.Add(-10 * time.Minute)})
	_ = metricRepo.Create(context.Background(), &activity.ServerMetric{ID: "m2", CPUPercent: 20, RecordedAt: base.Add(-5 * time.Minute)})
	_ = metricRepo.Create(context.Background(), &activity.ServerMetric{ID: "m3", CPUPercent: 30, RecordedAt: base.Add(5 * time.Minute)})

	metrics, err := querySvc.GetServerMetrics(context.Background(), base.Add(-6*time.Minute), base)
	require.NoError(t, err)
	assert.Len(t, metrics, 1)
	assert.InDelta(t, 20.0, metrics[0].CPUPercent, 0.01)
}

func TestPurgeOldMetrics(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, metricRepo, _, _, _ := newTestServices()

	base := time.Now()
	_ = metricRepo.Create(context.Background(), &activity.ServerMetric{ID: "old", CPUPercent: 1, RecordedAt: base.Add(-2 * time.Hour)})
	_ = metricRepo.Create(context.Background(), &activity.ServerMetric{ID: "new", CPUPercent: 2, RecordedAt: base})

	err := cmdSvc.PurgeOldMetrics(context.Background(), base.Add(-time.Hour))
	require.NoError(t, err)

	metrics, err := querySvc.GetServerMetrics(context.Background(), base.Add(-3*time.Hour), base.Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, metrics, 1)
	assert.Equal(t, "new", metrics[0].ID)
}

func TestPurgeOldAuditLogs(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, auditRepo, _, _ := newTestServices()

	base := time.Now()
	// Insert via the repo directly: CreateAuditEntry stamps CreatedAt=now,
	// which would defeat an "old entry" fixture.
	_ = auditRepo.Create(context.Background(), &activity.AuditLogEntry{ID: "old", Action: "login", CreatedAt: base.Add(-48 * time.Hour)})
	_ = auditRepo.Create(context.Background(), &activity.AuditLogEntry{ID: "new", Action: "login", CreatedAt: base})

	removed, err := cmdSvc.PurgeOldAuditLogs(context.Background(), base.Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	got, err := querySvc.GetAuditLog(context.Background(), activity.AuditLogFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0].ID)
}

func TestStartRetentionPurge_DisabledIsNoop(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, auditRepo, _, _ := newTestServices()
	_ = auditRepo.Create(context.Background(), &activity.AuditLogEntry{ID: "keep", Action: "login", CreatedAt: time.Now().Add(-1000 * time.Hour)})

	ctx := t.Context()
	// All windows disabled (<= 0): the goroutine still spawns (a window can be
	// enabled from the panel at runtime) but the pass must purge nothing.
	cmdSvc.StartRetentionPurge(ctx, func() (int, int, int) { return 0, 0, 0 })

	got, err := querySvc.GetAuditLog(context.Background(), activity.AuditLogFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 1, "disabled retention must not purge any entry")

	// A nil getter is the true no-op wiring (no goroutine at all).
	cmdSvc.StartRetentionPurge(ctx, nil)
}

func TestCreateAuditEntry_GeneratesID(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()

	entry := &activity.AuditLogEntry{
		UserID:       "u1",
		Action:       "login",
		ResourceType: "auth",
	}
	require.NoError(t, cmdSvc.CreateAuditEntry(context.Background(), entry))
	assert.NotEmpty(t, entry.ID)
}

func TestCreateAuditEntry_PreservesExistingID(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()

	entry := &activity.AuditLogEntry{
		ID:     "fixed-id",
		UserID: "u1",
		Action: "delete",
	}
	require.NoError(t, cmdSvc.CreateAuditEntry(context.Background(), entry))
	assert.Equal(t, "fixed-id", entry.ID)
}

// Under "connect as" the entry belongs to the impersonated user; the admin
// who acted is named too, and the caller's own details are left untouched.
func TestCreateAuditEntry_NamesTheImpersonatingAdmin(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()

	details := map[string]any{"reason": "cleanup"}
	ctx := activity.WithImpersonator(context.Background(), "admin-1")
	entry := &activity.AuditLogEntry{UserID: "u1", Action: "delete_playlist", Details: details}
	require.NoError(t, cmdSvc.CreateAuditEntry(ctx, entry))
	assert.Equal(t, "admin-1", entry.Details["impersonated_by"])
	assert.Equal(t, "cleanup", entry.Details["reason"])
	assert.NotContains(t, details, "impersonated_by")

	plain := &activity.AuditLogEntry{UserID: "u1", Action: "login"}
	require.NoError(t, cmdSvc.CreateAuditEntry(context.Background(), plain))
	assert.Nil(t, plain.Details)
}

func TestGetAuditLog_FilterByUserID(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, _, _, _ := newTestServices()

	_ = cmdSvc.CreateAuditEntry(context.Background(), &activity.AuditLogEntry{UserID: "u1", Action: "login"})
	_ = cmdSvc.CreateAuditEntry(context.Background(), &activity.AuditLogEntry{UserID: "u2", Action: "login"})
	_ = cmdSvc.CreateAuditEntry(context.Background(), &activity.AuditLogEntry{UserID: "u1", Action: "delete"})

	uid := "u1"
	entries, err := querySvc.GetAuditLog(context.Background(), activity.AuditLogFilter{UserID: &uid})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetAuditLog_FilterByAction(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, _, _, _ := newTestServices()

	_ = cmdSvc.CreateAuditEntry(context.Background(), &activity.AuditLogEntry{UserID: "u1", Action: "login"})
	_ = cmdSvc.CreateAuditEntry(context.Background(), &activity.AuditLogEntry{UserID: "u1", Action: "delete"})
	_ = cmdSvc.CreateAuditEntry(context.Background(), &activity.AuditLogEntry{UserID: "u2", Action: "login"})

	action := "login"
	entries, err := querySvc.GetAuditLog(context.Background(), activity.AuditLogFilter{Action: &action})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetUserStats(t *testing.T) {
	t.Parallel()
	_, querySvc, _, _, _, statsReader, _ := newTestServices()
	statsReader.UserStatsResult = &activity.UserStats{
		ProfileID:        "p1",
		TotalWatchTimeMs: 3600000,
		TotalPlayCount:   10,
	}

	stats, err := querySvc.GetUserStats(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, int64(3600000), stats.TotalWatchTimeMs)
	assert.Equal(t, 10, stats.TotalPlayCount)
}

func TestGetUserStats_Error(t *testing.T) {
	t.Parallel()
	_, querySvc, _, _, _, statsReader, _ := newTestServices()
	statsReader.Err = errors.New("stats unavailable")

	_, err := querySvc.GetUserStats(context.Background(), "p1")
	require.Error(t, err)
}

func TestGetServerStats(t *testing.T) {
	t.Parallel()
	_, querySvc, _, _, _, statsReader, _ := newTestServices()
	statsReader.ServerStatsResult = &activity.ServerStatsResult{
		ActiveStreams:    3,
		ActiveTranscodes: 1,
		TotalUsers:       5,
	}

	stats, err := querySvc.GetServerStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, stats.ActiveStreams)
	assert.Equal(t, 5, stats.TotalUsers)
}

func TestGetPlayHistory_FilterByProfile(t *testing.T) {
	t.Parallel()
	_, querySvc, _, _, _, _, playHistoryRepo := newTestServices()

	playHistoryRepo.Add(
		&activity.PlayHistoryEntry{ID: "h1", ProfileID: "p1", MediaItemID: "m1"},
		&activity.PlayHistoryEntry{ID: "h2", ProfileID: "p2", MediaItemID: "m2"},
		&activity.PlayHistoryEntry{ID: "h3", ProfileID: "p1", MediaItemID: "m3"},
	)

	entries, err := querySvc.GetPlayHistory(context.Background(), activity.PlayHistoryFilter{ProfileID: "p1"})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetPlayHistory_FilterByMediaItem(t *testing.T) {
	t.Parallel()
	_, querySvc, _, _, _, _, playHistoryRepo := newTestServices()

	playHistoryRepo.Add(
		&activity.PlayHistoryEntry{ID: "h1", ProfileID: "p1", MediaItemID: "m1"},
		&activity.PlayHistoryEntry{ID: "h2", ProfileID: "p2", MediaItemID: "m1"},
		&activity.PlayHistoryEntry{ID: "h3", ProfileID: "p1", MediaItemID: "m2"},
	)

	entries, err := querySvc.GetPlayHistory(context.Background(), activity.PlayHistoryFilter{MediaItemID: "m1"})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestWithAnalyticsReader(t *testing.T) {
	t.Parallel()
	_, querySvc, _, _, _, _, _ := newTestServices()

	reader := actfakes.NewFakeAnalyticsReader()
	reader.Data = &activity.AnalyticsData{
		TopMedia:       []activity.TopMediaItem{{MediaItemID: "m1", Title: "Movie", PlayCount: 5}},
		RecentActivity: []activity.RecentActivityEntry{},
	}

	appactivity.WithAnalyticsReader(reader)(querySvc)

	data, err := querySvc.GetAnalytics(context.Background())
	require.NoError(t, err)
	assert.Len(t, data.TopMedia, 1)
	assert.Equal(t, shared.MediaItemID("m1"), data.TopMedia[0].MediaItemID)
}

func TestGetMostPopular(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		popular []activity.MediaPlayCount
		limit   int
		wantLen int
		wantErr bool
	}{
		{
			name: "happy path",
			popular: []activity.MediaPlayCount{
				{MediaItemID: "m1", Title: "Movie A", PlayCount: 10},
				{MediaItemID: "m2", Title: "Movie B", PlayCount: 5},
			},
			limit:   10,
			wantLen: 2,
		},
		{
			name:    "empty results",
			popular: nil,
			limit:   10,
			wantLen: 0,
		},
		{
			name: "with limit smaller than results",
			popular: []activity.MediaPlayCount{
				{MediaItemID: "m1", Title: "A", PlayCount: 10},
				{MediaItemID: "m2", Title: "B", PlayCount: 5},
				{MediaItemID: "m3", Title: "C", PlayCount: 1},
			},
			limit:   2,
			wantLen: 2,
		},
		{
			name:    "error from reader",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, querySvc, _, _, _, statsReader, _ := newTestServices()

			if tc.wantErr {
				statsReader.Err = errors.New("stats error")
			} else {
				statsReader.MostPopular = tc.popular
			}

			result, err := querySvc.GetMostPopular(context.Background(), "", tc.limit)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, result, tc.wantLen)
		})
	}
}

func TestGetAnalytics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reader  *actfakes.FakeAnalyticsReader
		wantLen int
		wantErr bool
	}{
		{
			name: "happy path",
			reader: func() *actfakes.FakeAnalyticsReader {
				r := actfakes.NewFakeAnalyticsReader()
				r.Data = &activity.AnalyticsData{
					TopMedia: []activity.TopMediaItem{
						{MediaItemID: "m1", Title: "Movie", PlayCount: 10},
					},
					RecentActivity:     []activity.RecentActivityEntry{{UserID: "u1", Username: "alice"}},
					TotalPlaysToday:    5,
					TotalPlaysThisWeek: 20,
					ActiveUsersToday:   3,
				}
				return r
			}(),
			wantLen: 1,
		},
		{
			name:    "empty results",
			reader:  actfakes.NewFakeAnalyticsReader(),
			wantLen: 0,
		},
		{
			name: "nil reader returns empty data",
		},
		{
			name: "error from reader",
			reader: func() *actfakes.FakeAnalyticsReader {
				r := actfakes.NewFakeAnalyticsReader()
				r.Err = errors.New("analytics error")
				return r
			}(),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, querySvc, _, _, _, _, _ := newTestServices()

			if tc.reader != nil {
				appactivity.WithAnalyticsReader(tc.reader)(querySvc)
			}

			data, err := querySvc.GetAnalytics(context.Background())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, data)
			assert.Len(t, data.TopMedia, tc.wantLen)

			if tc.name == "nil reader returns empty data" {
				assert.Empty(t, data.TopMedia)
				assert.Empty(t, data.RecentActivity)
			}
			if tc.name == "happy path" {
				assert.Equal(t, 5, data.TotalPlaysToday)
				assert.Equal(t, 3, data.ActiveUsersToday)
			}
		})
	}
}

func TestGetLoginHistory_Pagination(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, _, _, _ := newTestServices()

	for range 5 {
		_ = cmdSvc.RecordLogin(context.Background(), &activity.LoginEntry{UserID: "u1", AuthMethod: "local", Success: true})
	}

	page1, err := querySvc.GetLoginHistory(context.Background(), "u1", 3, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 3)

	page2, err := querySvc.GetLoginHistory(context.Background(), "u1", 3, 3)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
}

// panicMetricRepo panics on Purge so a retention sweep panics mid-pass.
type panicMetricRepo struct{ called chan struct{} }

func (p *panicMetricRepo) Create(context.Context, *activity.ServerMetric) error { return nil }
func (p *panicMetricRepo) GetByTimeRange(context.Context, time.Time, time.Time) ([]*activity.ServerMetric, error) {
	return nil, nil
}
func (p *panicMetricRepo) TotalBandwidthBytes(context.Context) (int64, error) { return 0, nil }
func (p *panicMetricRepo) Purge(context.Context, time.Time) error {
	select {
	case p.called <- struct{}{}:
	default:
	}
	panic("simulated retention purge panic")
}

// TestStartRetentionPurge_RecoversPanicInPass proves the daily retention
// goroutine survives a panic in a pass (not just an error). Without the recover
// in runRetention, an unrecovered goroutine panic aborts the whole process;
// reaching the end of this test proves the panic was caught.
func TestStartRetentionPurge_RecoversPanicInPass(t *testing.T) {
	pm := &panicMetricRepo{called: make(chan struct{}, 1)}
	svc := appactivity.NewCommandService(
		actfakes.NewFakeLoginHistoryRepository(),
		pm,
		actfakes.NewFakeAuditLogRepository(),
		sharedfakes.NewFakeEventBus(),
	)

	// t.Context() is cancelled when the test ends, stopping the purge goroutine.
	ctx := t.Context()
	// metricsDays > 0 → the startup pass calls metrics.Purge, which panics.
	svc.StartRetentionPurge(ctx, func() (int, int, int) { return 0, 7, 0 })

	select {
	case <-pm.called:
	case <-time.After(2 * time.Second):
		t.Fatal("retention purge pass never ran")
	}
	// Grace for the deferred recover to unwind - an unrecovered panic would have
	// aborted the process before this returns.
	time.Sleep(100 * time.Millisecond)
}

// fakeQoEPurger signals each Purge call so a test can await the startup pass
// without sleeping.
type fakeQoEPurger struct {
	called chan time.Time
	err    error
}

func newFakeQoEPurger() *fakeQoEPurger {
	return &fakeQoEPurger{called: make(chan time.Time, 4)}
}

func (f *fakeQoEPurger) Purge(_ context.Context, before time.Time) (int64, error) {
	f.called <- before
	return 7, f.err
}

// playback_qoe_events is the busiest write path in the server and is otherwise
// append-only: without this sweep it grew unbounded from day one, despite
// migration 000103 sizing its indexes for a 30-day window it never enforced.
func TestStartRetentionPurge_PurgesQoEEvents(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()
	purger := newFakeQoEPurger()
	cmdSvc.WithQoEEventPurger(purger)

	cmdSvc.StartRetentionPurge(t.Context(), func() (int, int, int) { return 0, 0, 30 })

	select {
	case before := <-purger.called:
		age := time.Since(before)
		if age < 29*24*time.Hour || age > 31*24*time.Hour {
			t.Fatalf("cutoff must be ~30 days back, got %v", age)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("qoe retention sweep never ran")
	}
}

// A retention of 0 means "keep forever" for this table, exactly as it does for
// audit_log - an operator must be able to opt out of one without losing the
// others. With all three disabled no goroutine is even spawned.
func TestStartRetentionPurge_QoEDisabledKeepsForever(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()
	purger := newFakeQoEPurger()
	cmdSvc.WithQoEEventPurger(purger)

	cmdSvc.StartRetentionPurge(t.Context(), func() (int, int, int) { return 0, 0, 0 })

	select {
	case <-purger.called:
		t.Fatal("qoe retention 0 must keep forever, but a purge ran")
	case <-time.After(200 * time.Millisecond):
	}
}

// An unwired purger must not panic the daily sweep: it runs in a detached
// goroutine, so a nil deref there would take the whole process down and stop
// every future purge, not just this one.
func TestStartRetentionPurge_NilQoEPurgerIsSafe(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()
	cmdSvc.StartRetentionPurge(t.Context(), func() (int, int, int) { return 0, 0, 30 })
	time.Sleep(200 * time.Millisecond)
}

type fakeWebhookPurger struct{ called chan time.Time }

func (f *fakeWebhookPurger) Purge(_ context.Context, before time.Time) (int64, error) {
	f.called <- before
	return 4, nil
}

// The webhook delivery log is append-only and purely diagnostic - one row per
// delivery attempt, kept for the lifetime of the deployment because nothing
// ever deleted one.
func TestStartRetentionPurge_PurgesWebhookDeliveries(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()
	purger := &fakeWebhookPurger{called: make(chan time.Time, 4)}
	cmdSvc.WithWebhookDeliveryPurger(purger)

	// Every other retention disabled: the webhook sweep must still run, or an
	// operator who keeps audit logs forever silently keeps deliveries forever
	// too.
	cmdSvc.StartRetentionPurge(t.Context(), func() (int, int, int) { return 0, 0, 0 })

	select {
	case before := <-purger.called:
		age := time.Since(before)
		if age < 29*24*time.Hour || age > 31*24*time.Hour {
			t.Fatalf("with audit retention off the default 30-day window applies, got %v", age)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook delivery sweep never ran")
	}
}

// When the operator HAS set an audit window, deliveries follow it rather than
// the default - the two are the same class of diagnostic.
func TestStartRetentionPurge_WebhookFollowsAuditWindow(t *testing.T) {
	t.Parallel()
	cmdSvc, _, _, _, _, _, _ := newTestServices()
	purger := &fakeWebhookPurger{called: make(chan time.Time, 4)}
	cmdSvc.WithWebhookDeliveryPurger(purger)

	cmdSvc.StartRetentionPurge(t.Context(), func() (int, int, int) { return 7, 0, 0 })

	select {
	case before := <-purger.called:
		age := time.Since(before)
		if age < 6*24*time.Hour || age > 8*24*time.Hour {
			t.Fatalf("must follow the configured 7-day audit window, got %v", age)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook delivery sweep never ran")
	}
}
