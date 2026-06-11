package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/monitoring"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type MonitoringStatsRepository struct {
	db database.Database
}

func NewMonitoringStatsRepository(db database.Database) *MonitoringStatsRepository {
	return &MonitoringStatsRepository{db: db}
}

func (r *MonitoringStatsRepository) GetAuthStats(ctx context.Context, from time.Time, to time.Time) (*monitoring.AuthStats, error) {
	loginSuccess, err := r.count(ctx, loginHistoryCollection, withTimeRange(bson.M{"success": true}, from, to, "created_at"))
	if err != nil {
		return nil, err
	}
	loginFailure, err := r.count(ctx, loginHistoryCollection, withTimeRange(bson.M{"success": false}, from, to, "created_at"))
	if err != nil {
		return nil, err
	}
	activeSessions, err := r.count(ctx, sessionsCollection, bson.M{"revoked_at": bson.M{"$exists": false}})
	if err != nil {
		return nil, err
	}
	revokedSessions, err := r.count(ctx, sessionsCollection, withTimeRange(bson.M{"revoked_at": bson.M{"$exists": true}}, from, to, "revoked_at"))
	if err != nil {
		return nil, err
	}
	refreshCount, err := r.count(ctx, auditLogsCollection, withTimeRange(bson.M{"action": "auth.refresh"}, from, to, "created_at"))
	if err != nil {
		return nil, err
	}
	logoutCount, err := r.count(ctx, auditLogsCollection, withTimeRange(bson.M{"action": bson.M{"$in": []string{"auth.logout", "auth.logout_all"}}}, from, to, "created_at"))
	if err != nil {
		return nil, err
	}
	return &monitoring.AuthStats{
		LoginSuccessCount:   loginSuccess,
		LoginFailureCount:   loginFailure,
		ActiveSessionCount:  activeSessions,
		RevokedSessionCount: revokedSessions,
		RefreshCount:        refreshCount,
		LogoutCount:         logoutCount,
		From:                from,
		To:                  to,
	}, nil
}

func (r *MonitoringStatsRepository) count(ctx context.Context, collection string, filter bson.M) (int64, error) {
	return r.db.Count(ctx, collection, filter)
}

func withTimeRange(filter bson.M, from time.Time, to time.Time, field string) bson.M {
	if from.IsZero() && to.IsZero() {
		return filter
	}
	rangeFilter := bson.M{}
	if !from.IsZero() {
		rangeFilter["$gte"] = from
	}
	if !to.IsZero() {
		rangeFilter["$lte"] = to
	}
	filter[field] = rangeFilter
	return filter
}

var _ monitoring.AuthStatsRepository = (*MonitoringStatsRepository)(nil)
