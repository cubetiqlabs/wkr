package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/aspect-build/cubis-wkr/internal/model"
	"github.com/aspect-build/cubis-wkr/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrWorkerNotFound = errors.New("worker not found")
	ErrWorkerExists   = errors.New("worker with this name already exists")
	ErrUnauthorized   = errors.New("unauthorized")
)

type WorkerService struct {
	workerRepo     *repository.WorkerRepository
	deploymentRepo *repository.DeploymentRepository
	invocationRepo *repository.InvocationRepository
}

func NewWorkerService(
	workerRepo *repository.WorkerRepository,
	deploymentRepo *repository.DeploymentRepository,
	invocationRepo *repository.InvocationRepository,
) *WorkerService {
	return &WorkerService{
		workerRepo:     workerRepo,
		deploymentRepo: deploymentRepo,
		invocationRepo: invocationRepo,
	}
}

type CreateWorkerInput struct {
	Name       string            `json:"name" validate:"required,min=3,max=255"`
	Runtime    model.RuntimeType `json:"runtime" validate:"required,oneof=go javascript typescript"`
	EntryPoint string            `json:"entry_point"`
	Code       string            `json:"code" validate:"required"`
	EnvVars    model.JSONMap     `json:"env_vars"`
}

type UpdateWorkerInput struct {
	Code       *string       `json:"code"`
	EntryPoint *string       `json:"entry_point"`
	EnvVars    model.JSONMap `json:"env_vars"`
}

func (s *WorkerService) Create(ctx context.Context, ownerID uuid.UUID, input CreateWorkerInput) (*model.Worker, error) {
	if existing, _ := s.workerRepo.GetByName(ctx, input.Name); existing != nil {
		return nil, ErrWorkerExists
	}

	entryPoint := "main"
	if input.EntryPoint != "" {
		entryPoint = input.EntryPoint
	}

	w := &model.Worker{
		Name:       input.Name,
		Runtime:    input.Runtime,
		EntryPoint: entryPoint,
		Code:       input.Code,
		CodeHash:   hashCode(input.Code),
		Version:    1,
		Status:     model.WorkerStatusActive,
		EnvVars:    input.EnvVars,
		OwnerID:    ownerID,
	}

	if err := s.workerRepo.Create(ctx, w); err != nil {
		return nil, fmt.Errorf("failed to create worker: %w", err)
	}

	// Create initial deployment
	dep := &model.Deployment{
		WorkerID:   w.ID,
		Version:    1,
		CodeHash:   w.CodeHash,
		Status:     model.DeploymentActive,
		DeployedBy: ownerID,
	}
	_ = s.deploymentRepo.Create(ctx, dep)

	return w, nil
}

func (s *WorkerService) Get(ctx context.Context, id uuid.UUID) (*model.Worker, error) {
	w, err := s.workerRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrWorkerNotFound
	}
	return w, nil
}

func (s *WorkerService) List(ctx context.Context, ownerID uuid.UUID, page, pageSize int) ([]model.Worker, int64, error) {
	offset := (page - 1) * pageSize
	return s.workerRepo.ListByOwner(ctx, ownerID, offset, pageSize)
}

func (s *WorkerService) Update(ctx context.Context, id, ownerID uuid.UUID, input UpdateWorkerInput) (*model.Worker, error) {
	w, err := s.workerRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrWorkerNotFound
	}
	if w.OwnerID != ownerID {
		return nil, ErrUnauthorized
	}

	if input.Code != nil {
		w.Code = *input.Code
		w.CodeHash = hashCode(*input.Code)
		w.Version++
	}
	if input.EntryPoint != nil {
		w.EntryPoint = *input.EntryPoint
	}
	if input.EnvVars != nil {
		w.EnvVars = input.EnvVars
	}

	if err := s.workerRepo.Update(ctx, w); err != nil {
		return nil, fmt.Errorf("failed to update worker: %w", err)
	}

	if input.Code != nil {
		dep := &model.Deployment{
			WorkerID:   w.ID,
			Version:    w.Version,
			CodeHash:   w.CodeHash,
			Status:     model.DeploymentActive,
			DeployedBy: ownerID,
		}
		_ = s.deploymentRepo.Create(ctx, dep)
	}

	return w, nil
}

func (s *WorkerService) Delete(ctx context.Context, id, ownerID uuid.UUID) error {
	w, err := s.workerRepo.GetByID(ctx, id)
	if err != nil {
		return ErrWorkerNotFound
	}
	if w.OwnerID != ownerID {
		return ErrUnauthorized
	}
	return s.workerRepo.Delete(ctx, id)
}

func (s *WorkerService) GetByName(ctx context.Context, name string) (*model.Worker, error) {
	w, err := s.workerRepo.GetActiveByName(ctx, name)
	if err != nil {
		return nil, ErrWorkerNotFound
	}
	return w, nil
}

func (s *WorkerService) RecordInvocation(ctx context.Context, inv *model.Invocation) {
	_ = s.invocationRepo.Create(ctx, inv)
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", h)
}
