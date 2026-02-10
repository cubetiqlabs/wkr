package repository

import (
	"context"

	"github.com/aspect-build/cubis-wkr/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvocationRepository struct {
	db *gorm.DB
}

func NewInvocationRepository(db *gorm.DB) *InvocationRepository {
	return &InvocationRepository{db: db}
}

func (r *InvocationRepository) Create(ctx context.Context, inv *model.Invocation) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

func (r *InvocationRepository) ListByWorker(ctx context.Context, workerID uuid.UUID, offset, limit int) ([]model.Invocation, int64, error) {
	var invocations []model.Invocation
	var total int64

	q := r.db.WithContext(ctx).Model(&model.Invocation{}).Where("worker_id = ?", workerID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Offset(offset).Limit(limit).Order("created_at DESC").Find(&invocations).Error; err != nil {
		return nil, 0, err
	}
	return invocations, total, nil
}
