package repository

import (
	"context"
	"time"

	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type QuotaRepository struct {
	db *gorm.DB
}

func NewQuotaRepository(db *gorm.DB) *QuotaRepository {
	return &QuotaRepository{db: db}
}

func (r *QuotaRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.AccountQuota, error) {
	var q model.AccountQuota
	err := r.db.WithContext(ctx).First(&q, "user_id = ?", userID).Error
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *QuotaRepository) Upsert(ctx context.Context, q *model.AccountQuota) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"plan", "max_requests_per_day", "max_exec_time_per_day", "max_bandwidth_per_day", "max_workers", "updated_at"}),
		}).Create(q).Error
}

func (r *QuotaRepository) GetTodayUsage(ctx context.Context, userID uuid.UUID) (*model.UsageMetrics, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var m model.UsageMetrics
	err := r.db.WithContext(ctx).First(&m, "user_id = ? AND date = ?", userID, today).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *QuotaRepository) IncrementUsage(ctx context.Context, userID uuid.UUID, execTimeMs, requestBytes, responseBytes int64, isError bool) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	errorInc := int64(0)
	if isError {
		errorInc = 1
	}

	// Upsert: create row if not exists, otherwise increment
	id := uuid.Must(uuid.NewV7())
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO usage_metrics (id, user_id, date, request_count, total_exec_time, total_bandwidth, error_count, updated_at)
		VALUES (?, ?, ?, 1, ?, ?, ?, NOW())
		ON CONFLICT (user_id, date)
		DO UPDATE SET
			request_count = usage_metrics.request_count + 1,
			total_exec_time = usage_metrics.total_exec_time + EXCLUDED.total_exec_time,
			total_bandwidth = usage_metrics.total_bandwidth + EXCLUDED.total_bandwidth,
			error_count = usage_metrics.error_count + EXCLUDED.error_count,
			updated_at = NOW()
	`, id, userID, today, execTimeMs, requestBytes+responseBytes, errorInc).Error
}

func (r *QuotaRepository) GetUsageRange(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]model.UsageMetrics, error) {
	var metrics []model.UsageMetrics
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND date >= ? AND date <= ?", userID, from, to).
		Order("date ASC").
		Find(&metrics).Error
	return metrics, err
}
