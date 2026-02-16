package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base provides common fields for all models.
type Base struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.Must(uuid.NewV7())
	}
	return nil
}

// Worker represents a serverless function deployment.
type Worker struct {
	Base
	Name           string        `gorm:"size:255;not null;uniqueIndex:idx_owner_worker_name" json:"name" validate:"required,min=3,max=255"`
	Runtime        RuntimeType   `gorm:"size:20;not null" json:"runtime" validate:"required,oneof=go javascript typescript python"`
	RuntimeVersion string        `gorm:"size:20;not null;default:''" json:"runtime_version,omitempty"` // e.g. "1.24", "3.12", "22"
	EntryPoint     string        `gorm:"size:255;not null;default:'main'" json:"entry_point"`
	Code           string        `gorm:"type:text;not null" json:"code" validate:"required"`
	CodeHash       string        `gorm:"size:64;not null" json:"code_hash"`
	Dependencies   string        `gorm:"type:text;not null;default:''" json:"dependencies,omitempty"`    // package manifest content
	PackageManager string        `gorm:"size:20;not null;default:''" json:"package_manager,omitempty"`   // npm, yarn, pnpm, deno, pip, uv, go
	Version        int           `gorm:"not null;default:1" json:"version"`
	Status         WorkerStatus  `gorm:"size:20;not null;default:'inactive'" json:"status"`
	Timeout        time.Duration `gorm:"not null;default:30000000000" json:"timeout"` // 30s default
	MemoryLimit    int           `gorm:"not null;default:128" json:"memory_limit"`    // MB
	EnvVars        JSONMap       `gorm:"type:jsonb;default:'{}'" json:"env_vars"`
	OwnerID        uuid.UUID     `gorm:"type:uuid;not null;index;uniqueIndex:idx_owner_worker_name" json:"owner_id"`
	Owner          *User         `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
}

type RuntimeType string

const (
	RuntimeGo         RuntimeType = "go"
	RuntimeJavaScript RuntimeType = "javascript"
	RuntimeTypeScript RuntimeType = "typescript"
	RuntimePython     RuntimeType = "python"
)

type WorkerStatus string

const (
	WorkerStatusActive   WorkerStatus = "active"
	WorkerStatusInactive WorkerStatus = "inactive"
	WorkerStatusError    WorkerStatus = "error"
)

// Deployment tracks each deployment of a worker.
type Deployment struct {
	Base
	WorkerID       uuid.UUID        `gorm:"type:uuid;not null;index" json:"worker_id"`
	Worker         *Worker          `gorm:"foreignKey:WorkerID" json:"worker,omitempty"`
	Version        int              `gorm:"not null" json:"version"`
	Code           string           `gorm:"type:text;not null" json:"code,omitempty"`
	EntryPoint     string           `gorm:"size:255;not null;default:'main'" json:"entry_point"`
	CodeHash       string           `gorm:"size:64;not null" json:"code_hash"`
	Dependencies   string           `gorm:"type:text;not null;default:''" json:"dependencies,omitempty"`
	EnvVars        JSONMap          `gorm:"type:jsonb;default:'{}'" json:"env_vars,omitempty"`
	Status         DeploymentStatus `gorm:"size:20;not null;default:'pending'" json:"status"`
	DeployedBy     uuid.UUID        `gorm:"type:uuid;not null" json:"deployed_by"`
}

type DeploymentStatus string

const (
	DeploymentPending  DeploymentStatus = "pending"
	DeploymentActive   DeploymentStatus = "active"
	DeploymentFailed   DeploymentStatus = "failed"
	DeploymentRolledBack DeploymentStatus = "rolled_back"
)

// Invocation logs each worker execution.
type Invocation struct {
	ID            uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	WorkerID      uuid.UUID     `gorm:"type:uuid;not null;index" json:"worker_id"`
	OwnerID       uuid.UUID     `gorm:"type:uuid;not null;index" json:"owner_id"`
	RequestID     string        `gorm:"size:64;not null;index" json:"request_id"`
	NodeID        string        `gorm:"size:64" json:"node_id"`
	Region        string        `gorm:"size:32" json:"region,omitempty"`
	Duration      time.Duration `json:"duration"`
	StatusCode    int           `json:"status_code"`
	MemoryUsed    int           `json:"memory_used"`
	RequestBytes  int64         `json:"request_bytes"`
	ResponseBytes int64         `json:"response_bytes"`
	Method        string        `gorm:"size:10" json:"method"`
	Path          string        `gorm:"size:2048" json:"path"`
	ClientIP      string        `gorm:"size:45" json:"client_ip"`
	UserAgent     string        `gorm:"size:512" json:"user_agent,omitempty"`
	Error         string        `gorm:"type:text" json:"error,omitempty"`
	Logs          StringSlice   `gorm:"type:jsonb;default:'[]'" json:"logs,omitempty"`
	StackTrace    string        `gorm:"type:text" json:"stack_trace,omitempty"`
	CreatedAt     time.Time     `gorm:"autoCreateTime;index" json:"created_at"`
}

func (i *Invocation) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.Must(uuid.NewV7())
	}
	return nil
}
