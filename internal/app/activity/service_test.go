package activity_test

import (
	"context"
	"testing"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	"github.com/gravity-zero/evey/internal/domain/activity"
	actfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
)

func TestRecordLogin(t *testing.T) {
	loginRepo := actfakes.NewFakeLoginHistoryRepository()
	metricRepo := actfakes.NewFakeServerMetricRepository()
	auditRepo := actfakes.NewFakeAuditLogRepository()
	statsReader := actfakes.NewFakePlaySessionStatsReader()
	bus := sharedfakes.NewFakeEventBus()

	playHistoryRepo := actfakes.NewFakePlayHistoryRepository()
	svc := appactivity.NewService(loginRepo, metricRepo, auditRepo, statsReader, playHistoryRepo, bus)

	err := svc.RecordLogin(context.Background(), &activity.LoginEntry{
		ID:         "l1",
		UserID:     "u1",
		ClientIP:   "127.0.0.1",
		AuthMethod: "local",
		Success:    true,
	})
	assert.NoError(t, err)

	history, err := svc.GetLoginHistory(context.Background(), "u1", 10, 0)
	assert.NoError(t, err)
	assert.Len(t, history, 1)
}
