package repository

import (
	"context"

	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DeploymentRepository struct {
	db *gorm.DB
}

func NewDeploymentRepository(db *gorm.DB) *DeploymentRepository {
	return &DeploymentRepository{db: db}
}

func (r *DeploymentRepository) Create(ctx context.Context, d *model.Deployment) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *DeploymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Deployment, error) {
	var d model.Deployment
	err := r.db.WithContext(ctx).Preload("Worker").First(&d, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeploymentRepository) ListByWorker(ctx context.Context, workerID uuid.UUID, limit int) ([]model.Deployment, error) {
	var deployments []model.Deployment
	err := r.db.WithContext(ctx).
		Where("worker_id = ?", workerID).
		Order("created_at DESC").
		Limit(limit).
		Find(&deployments).Error
	return deployments, err
}

func (r *DeploymentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status model.DeploymentStatus) error {
	return r.db.WithContext(ctx).Model(&model.Deployment{}).Where("id = ?", id).Update("status", status).Error
}
