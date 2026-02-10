package model

import (
	"time"

	"github.com/google/uuid"
)

// EdgeNode represents a node in the distributed execution cluster.
type EdgeNode struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	NodeID    string     `gorm:"uniqueIndex;size:255;not null" json:"node_id"`
	Region    string     `gorm:"size:64;not null;index" json:"region"`
	Role      NodeRole   `gorm:"size:20;not null" json:"role"`
	Status    NodeStatus `gorm:"size:20;not null;default:'offline'" json:"status"`
	Endpoint  string     `gorm:"size:512;not null" json:"endpoint"`
	ActiveWorkers int   `gorm:"not null;default:0" json:"active_workers"`
	MaxWorkers    int   `gorm:"not null;default:100" json:"max_workers"`
	LastHeartbeat time.Time `gorm:"index" json:"last_heartbeat"`
	Metadata  JSONMap    `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

type NodeRole string

const (
	NodeRoleControl NodeRole = "control"
	NodeRoleEdge    NodeRole = "edge"
)

type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusDraining NodeStatus = "draining"
)

// AuditLog records security-relevant events for forensics.
type AuditLog struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Event     string    `gorm:"size:64;not null;index" json:"event"`
	ActorID   uuid.UUID `gorm:"type:uuid;index" json:"actor_id"`
	WorkerID  uuid.UUID `gorm:"type:uuid;index" json:"worker_id,omitempty"`
	NodeID    string    `gorm:"size:255;index" json:"node_id"`
	IP        string    `gorm:"size:45" json:"ip"`
	Detail    string    `gorm:"type:text" json:"detail"`
	Severity  string    `gorm:"size:20;not null;default:'info'" json:"severity"` // info, warn, critical
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}
