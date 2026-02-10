package security

import (
	"context"
	"time"

	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Auditor records security events to the database and structured log.
type Auditor struct {
	db      *gorm.DB
	enabled bool
	nodeID  string
}

func NewAuditor(db *gorm.DB, enabled bool, nodeID string) *Auditor {
	return &Auditor{db: db, enabled: enabled, nodeID: nodeID}
}

func (a *Auditor) Log(_ context.Context, event, severity, detail, ip string, actorID, workerID uuid.UUID) {
	logger.Info("audit",
		zap.String("event", event),
		zap.String("severity", severity),
		zap.String("detail", detail),
		zap.String("node_id", a.nodeID),
	)

	if !a.enabled {
		return
	}

	entry := &model.AuditLog{
		Event:    event,
		ActorID:  actorID,
		WorkerID: workerID,
		NodeID:   a.nodeID,
		IP:       ip,
		Detail:   detail,
		Severity: severity,
	}
	// Use independent context — the request context may be cancelled
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.db.WithContext(ctx).Create(entry).Error; err != nil {
			logger.Error("audit log write failed", zap.Error(err))
		}
	}()
}
