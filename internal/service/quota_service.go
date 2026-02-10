package service

import (
	"context"
	"errors"
	"time"

	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/cubetiqlabs/wkr/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrQuotaRequestsExceeded  = errors.New("daily request limit exceeded")
	ErrQuotaExecTimeExceeded  = errors.New("daily execution time limit exceeded")
	ErrQuotaBandwidthExceeded = errors.New("daily bandwidth limit exceeded")
	ErrQuotaWorkersExceeded   = errors.New("maximum workers limit exceeded")
)

type QuotaService struct {
	quotaRepo  *repository.QuotaRepository
	workerRepo *repository.WorkerRepository
}

func NewQuotaService(quotaRepo *repository.QuotaRepository, workerRepo *repository.WorkerRepository) *QuotaService {
	return &QuotaService{quotaRepo: quotaRepo, workerRepo: workerRepo}
}

// EnsureQuota creates a default quota for a user if none exists.
func (s *QuotaService) EnsureQuota(ctx context.Context, userID uuid.UUID) (*model.AccountQuota, error) {
	q, err := s.quotaRepo.GetByUserID(ctx, userID)
	if err == nil {
		return q, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	q = model.DefaultQuota(userID, model.PlanFree)
	if err := s.quotaRepo.Upsert(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// CheckInvocationAllowed verifies the user hasn't exceeded daily limits.
func (s *QuotaService) CheckInvocationAllowed(ctx context.Context, userID uuid.UUID) error {
	quota, err := s.EnsureQuota(ctx, userID)
	if err != nil {
		return err
	}

	usage, err := s.quotaRepo.GetTodayUsage(ctx, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if usage == nil {
		return nil // no usage yet today
	}

	if quota.MaxRequestsPerDay > 0 && usage.RequestCount >= quota.MaxRequestsPerDay {
		return ErrQuotaRequestsExceeded
	}
	if quota.MaxExecTimePerDay > 0 && usage.TotalExecTime/1000 >= quota.MaxExecTimePerDay {
		return ErrQuotaExecTimeExceeded
	}
	if quota.MaxBandwidthPerDay > 0 && usage.TotalBandwidth >= quota.MaxBandwidthPerDay {
		return ErrQuotaBandwidthExceeded
	}
	return nil
}

// CheckWorkerLimit verifies the user can create more workers.
func (s *QuotaService) CheckWorkerLimit(ctx context.Context, userID uuid.UUID) error {
	quota, err := s.EnsureQuota(ctx, userID)
	if err != nil {
		return err
	}
	if quota.MaxWorkers < 0 {
		return nil // unlimited
	}
	_, count, err := s.workerRepo.ListByOwner(ctx, userID, 0, 1)
	if err != nil {
		return err
	}
	if int(count) >= quota.MaxWorkers {
		return ErrQuotaWorkersExceeded
	}
	return nil
}

// RecordUsage increments daily usage counters after an invocation.
func (s *QuotaService) RecordUsage(ctx context.Context, userID uuid.UUID, execTimeMs, reqBytes, respBytes int64, isError bool) {
	if err := s.quotaRepo.IncrementUsage(ctx, userID, execTimeMs, reqBytes, respBytes, isError); err != nil {
		logger.Error("failed to record usage", zap.Error(err), zap.String("user_id", userID.String()))
	}
}

// GetUsage returns today's usage for a user.
func (s *QuotaService) GetUsage(ctx context.Context, userID uuid.UUID) (*model.UsageMetrics, error) {
	return s.quotaRepo.GetTodayUsage(ctx, userID)
}

// GetQuota returns the user's quota.
func (s *QuotaService) GetQuota(ctx context.Context, userID uuid.UUID) (*model.AccountQuota, error) {
	return s.EnsureQuota(ctx, userID)
}

// GetUsageRange returns usage metrics for a date range.
func (s *QuotaService) GetUsageRange(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]model.UsageMetrics, error) {
	return s.quotaRepo.GetUsageRange(ctx, userID, from, to)
}

// UpdatePlan changes a user's plan and updates quota limits.
func (s *QuotaService) UpdatePlan(ctx context.Context, userID uuid.UUID, plan model.PlanTier) error {
	q := model.DefaultQuota(userID, plan)
	return s.quotaRepo.Upsert(ctx, q)
}
