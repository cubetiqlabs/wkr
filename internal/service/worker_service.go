package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/cubetiqlabs/wkr/internal/crypto"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/cubetiqlabs/wkr/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
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
	encryptionKey  string
}

func NewWorkerService(
	workerRepo *repository.WorkerRepository,
	deploymentRepo *repository.DeploymentRepository,
	invocationRepo *repository.InvocationRepository,
	encryptionKey string,
) *WorkerService {
	return &WorkerService{
		workerRepo:     workerRepo,
		deploymentRepo: deploymentRepo,
		invocationRepo: invocationRepo,
		encryptionKey:  encryptionKey,
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

	// Encrypt env vars at rest
	envVars := input.EnvVars
	if len(envVars) > 0 && s.encryptionKey != "" {
		encrypted, err := crypto.EncryptMap(envVars, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt env vars: %w", err)
		}
		envVars = encrypted
	}

	w := &model.Worker{
		Name:       input.Name,
		Runtime:    input.Runtime,
		EntryPoint: entryPoint,
		Code:       input.Code,
		CodeHash:   hashCode(input.Code),
		Version:    1,
		Status:     model.WorkerStatusActive,
		EnvVars:    envVars,
		OwnerID:    ownerID,
	}

	if err := s.workerRepo.Create(ctx, w); err != nil {
		return nil, fmt.Errorf("failed to create worker: %w", err)
	}

	dep := &model.Deployment{
		WorkerID:   w.ID,
		Version:    1,
		CodeHash:   w.CodeHash,
		Status:     model.DeploymentActive,
		DeployedBy: ownerID,
	}
	if err := s.deploymentRepo.Create(ctx, dep); err != nil {
		logger.Error("failed to record deployment", zap.Error(err))
	}

	// Return with decrypted env vars for the response
	w.EnvVars = input.EnvVars
	return w, nil
}

func (s *WorkerService) Get(ctx context.Context, id uuid.UUID) (*model.Worker, error) {
	w, err := s.workerRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrWorkerNotFound
	}
	// Mask env var values in response (show only that they exist)
	w.EnvVars = maskEnvVars(w.EnvVars)
	return w, nil
}

func (s *WorkerService) List(ctx context.Context, ownerID uuid.UUID, page, pageSize int) ([]model.Worker, int64, error) {
	offset := (page - 1) * pageSize
	workers, total, err := s.workerRepo.ListByOwner(ctx, ownerID, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	for i := range workers {
		workers[i].EnvVars = maskEnvVars(workers[i].EnvVars)
	}
	return workers, total, nil
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
		if s.encryptionKey != "" {
			encrypted, err := crypto.EncryptMap(input.EnvVars, s.encryptionKey)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt env vars: %w", err)
			}
			w.EnvVars = encrypted
		} else {
			w.EnvVars = input.EnvVars
		}
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
		if err := s.deploymentRepo.Create(ctx, dep); err != nil {
			logger.Error("failed to record deployment", zap.Error(err))
		}
	}

	w.EnvVars = maskEnvVars(w.EnvVars)
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

// GetByName returns a worker with decrypted env vars (for execution).
func (s *WorkerService) GetByName(ctx context.Context, name string) (*model.Worker, error) {
	w, err := s.workerRepo.GetActiveByName(ctx, name)
	if err != nil {
		return nil, ErrWorkerNotFound
	}
	// Decrypt env vars for runtime execution
	if len(w.EnvVars) > 0 && s.encryptionKey != "" {
		decrypted, err := crypto.DecryptMap(w.EnvVars, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt env vars: %w", err)
		}
		w.EnvVars = decrypted
	}
	return w, nil
}

// GetWorkerByNameForOwner returns a worker by name if owned by the given user.
func (s *WorkerService) GetWorkerByNameForOwner(ctx context.Context, name string, ownerID uuid.UUID) (*model.Worker, error) {
	w, err := s.workerRepo.GetByName(ctx, name)
	if err != nil {
		return nil, ErrWorkerNotFound
	}
	if w.OwnerID != ownerID {
		return nil, ErrUnauthorized
	}
	return w, nil
}

func (s *WorkerService) UpdateByName(ctx context.Context, name string, ownerID uuid.UUID, input UpdateWorkerInput) (*model.Worker, error) {
	w, err := s.GetWorkerByNameForOwner(ctx, name, ownerID)
	if err != nil {
		return nil, err
	}
	return s.Update(ctx, w.ID, ownerID, input)
}

func (s *WorkerService) DeleteByName(ctx context.Context, name string, ownerID uuid.UUID) error {
	w, err := s.GetWorkerByNameForOwner(ctx, name, ownerID)
	if err != nil {
		return err
	}
	return s.workerRepo.Delete(ctx, w.ID)
}

func (s *WorkerService) RecordInvocation(ctx context.Context, inv *model.Invocation) {
	_ = s.invocationRepo.Create(ctx, inv)
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", h)
}

// maskEnvVars replaces values with "***" for API responses.
func maskEnvVars(m model.JSONMap) model.JSONMap {
	masked := make(model.JSONMap, len(m))
	for k := range m {
		masked[k] = "***"
	}
	return masked
}
