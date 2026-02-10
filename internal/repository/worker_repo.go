package repository

import (
	"context"

	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WorkerRepository struct {
	db *gorm.DB
}

func NewWorkerRepository(db *gorm.DB) *WorkerRepository {
	return &WorkerRepository{db: db}
}

func (r *WorkerRepository) Create(ctx context.Context, w *model.Worker) error {
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *WorkerRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Worker, error) {
	var w model.Worker
	err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WorkerRepository) GetByName(ctx context.Context, name string) (*model.Worker, error) {
	var w model.Worker
	err := r.db.WithContext(ctx).First(&w, "name = ?", name).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WorkerRepository) ListByOwner(ctx context.Context, ownerID uuid.UUID, offset, limit int) ([]model.Worker, int64, error) {
	var workers []model.Worker
	var total int64

	q := r.db.WithContext(ctx).Model(&model.Worker{}).Where("owner_id = ?", ownerID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Offset(offset).Limit(limit).Order("created_at DESC").Find(&workers).Error; err != nil {
		return nil, 0, err
	}
	return workers, total, nil
}

func (r *WorkerRepository) Update(ctx context.Context, w *model.Worker) error {
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *WorkerRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Worker{}, "id = ?", id).Error
}

func (r *WorkerRepository) GetActiveByName(ctx context.Context, name string) (*model.Worker, error) {
	var w model.Worker
	err := r.db.WithContext(ctx).First(&w, "name = ? AND status = ?", name, model.WorkerStatusActive).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}
