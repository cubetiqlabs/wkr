package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AccountQuota defines usage limits for a user account.
type AccountQuota struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID             uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	User               *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Plan               PlanTier  `gorm:"size:20;not null;default:'free'" json:"plan"`
	MaxRequestsPerDay  int64     `gorm:"not null;default:1000" json:"max_requests_per_day"`
	MaxExecTimePerDay  int64     `gorm:"not null;default:3600" json:"max_exec_time_per_day"`
	MaxBandwidthPerDay int64     `gorm:"not null;default:104857600" json:"max_bandwidth_per_day"`
	MaxWorkers         int       `gorm:"not null;default:5" json:"max_workers"`
	CreatedAt          time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (q *AccountQuota) BeforeCreate(tx *gorm.DB) error {
	if q.ID == uuid.Nil {
		q.ID = uuid.Must(uuid.NewV7())
	}
	return nil
}

type PlanTier string

const (
	PlanFree       PlanTier = "free"
	PlanPro        PlanTier = "pro"
	PlanEnterprise PlanTier = "enterprise"
)

// DefaultQuota returns default limits for a plan tier.
func DefaultQuota(userID uuid.UUID, plan PlanTier) *AccountQuota {
	q := &AccountQuota{UserID: userID, Plan: plan}
	switch plan {
	case PlanPro:
		q.MaxRequestsPerDay = 50000
		q.MaxExecTimePerDay = 36000
		q.MaxBandwidthPerDay = 1 << 30
		q.MaxWorkers = 50
	case PlanEnterprise:
		q.MaxRequestsPerDay = -1
		q.MaxExecTimePerDay = -1
		q.MaxBandwidthPerDay = -1
		q.MaxWorkers = -1
	default:
		q.MaxRequestsPerDay = 1000
		q.MaxExecTimePerDay = 3600
		q.MaxBandwidthPerDay = 100 << 20
		q.MaxWorkers = 5
	}
	return q
}

// UsageMetrics tracks daily usage per user (one row per user per day).
type UsageMetrics struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index:idx_usage_user_date,unique" json:"user_id"`
	Date           time.Time `gorm:"type:date;not null;index:idx_usage_user_date,unique" json:"date"`
	RequestCount   int64     `gorm:"not null;default:0" json:"request_count"`
	TotalExecTime  int64     `gorm:"not null;default:0" json:"total_exec_time"`
	TotalBandwidth int64     `gorm:"not null;default:0" json:"total_bandwidth"`
	ErrorCount     int64     `gorm:"not null;default:0" json:"error_count"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (u *UsageMetrics) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.Must(uuid.NewV7())
	}
	return nil
}
