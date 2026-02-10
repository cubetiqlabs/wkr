package edge

import (
	"context"
	"sync"
	"time"

	"github.com/cubetiqlabs/wkr/internal/config"
	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/metrics"
	"github.com/cubetiqlabs/wkr/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Registry manages edge node registration, heartbeats, and discovery.
type Registry struct {
	db     *gorm.DB
	cfg    config.EdgeConfig
	mu     sync.RWMutex
	nodes  map[string]*model.EdgeNode // nodeID -> node (in-memory cache)
	stopCh chan struct{}
}

func NewRegistry(db *gorm.DB, cfg config.EdgeConfig) *Registry {
	return &Registry{
		db:     db,
		cfg:    cfg,
		nodes:  make(map[string]*model.EdgeNode),
		stopCh: make(chan struct{}),
	}
}

// RegisterSelf registers or updates this node in the cluster.
func (r *Registry) RegisterSelf(endpoint string, maxWorkers int) error {
	node := &model.EdgeNode{
		NodeID:     r.cfg.NodeID,
		Region:     r.cfg.Region,
		Role:       model.NodeRole(r.cfg.Role),
		Status:     model.NodeStatusOnline,
		Endpoint:   endpoint,
		MaxWorkers: maxWorkers,
		LastHeartbeat: time.Now(),
		Metadata: model.JSONMap{
			"version": "0.1.0",
		},
	}

	result := r.db.Where("node_id = ?", r.cfg.NodeID).First(&model.EdgeNode{})
	if result.Error == gorm.ErrRecordNotFound {
		return r.db.Create(node).Error
	}
	return r.db.Model(&model.EdgeNode{}).Where("node_id = ?", r.cfg.NodeID).Updates(map[string]interface{}{
		"status":         model.NodeStatusOnline,
		"endpoint":       endpoint,
		"max_workers":    maxWorkers,
		"last_heartbeat": time.Now(),
		"region":         r.cfg.Region,
		"role":           r.cfg.Role,
	}).Error
}

// StartHeartbeat begins periodic heartbeat and node discovery.
func (r *Registry) StartHeartbeat() {
	go func() {
		ticker := time.NewTicker(r.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.sendHeartbeat()
				r.refreshNodes()
				r.detectFailures()
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *Registry) sendHeartbeat() {
	r.db.Model(&model.EdgeNode{}).Where("node_id = ?", r.cfg.NodeID).Updates(map[string]interface{}{
		"last_heartbeat": time.Now(),
		"status":         model.NodeStatusOnline,
	})
	metrics.EdgeNodeStatus.WithLabelValues(r.cfg.NodeID, r.cfg.Region, r.cfg.Role).Set(1)
}

func (r *Registry) refreshNodes() {
	var nodes []model.EdgeNode
	if err := r.db.Where("status != ?", model.NodeStatusOffline).Find(&nodes).Error; err != nil {
		logger.Error("failed to refresh edge nodes", zap.Error(err))
		return
	}
	r.mu.Lock()
	r.nodes = make(map[string]*model.EdgeNode, len(nodes))
	for i := range nodes {
		r.nodes[nodes[i].NodeID] = &nodes[i]
	}
	r.mu.Unlock()
}

func (r *Registry) detectFailures() {
	threshold := time.Now().Add(-r.cfg.FailoverTimeout)
	result := r.db.Model(&model.EdgeNode{}).
		Where("status = ? AND last_heartbeat < ? AND node_id != ?", model.NodeStatusOnline, threshold, r.cfg.NodeID).
		Update("status", model.NodeStatusOffline)

	if result.RowsAffected > 0 {
		logger.Warn("edge nodes marked offline due to missed heartbeats",
			zap.Int64("count", result.RowsAffected),
		)
	}
}

// GetHealthyNodes returns all online nodes, optionally filtered by region.
func (r *Registry) GetHealthyNodes(region string) []*model.EdgeNode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.EdgeNode
	for _, n := range r.nodes {
		if n.Status == model.NodeStatusOnline {
			if region == "" || n.Region == region {
				result = append(result, n)
			}
		}
	}
	return result
}

// GetNodeByID returns a specific node.
func (r *Registry) GetNodeByID(nodeID string) *model.EdgeNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes[nodeID]
}

// UpdateActiveWorkers updates the active worker count for this node.
func (r *Registry) UpdateActiveWorkers(count int) {
	r.db.Model(&model.EdgeNode{}).Where("node_id = ?", r.cfg.NodeID).
		Update("active_workers", count)
	metrics.ActiveWorkers.WithLabelValues(r.cfg.NodeID, r.cfg.Region).Set(float64(count))
}

// Shutdown marks this node as offline.
func (r *Registry) Shutdown() {
	close(r.stopCh)
	r.db.Model(&model.EdgeNode{}).Where("node_id = ?", r.cfg.NodeID).
		Update("status", model.NodeStatusOffline)
	metrics.EdgeNodeStatus.WithLabelValues(r.cfg.NodeID, r.cfg.Region, r.cfg.Role).Set(0)
}

// ListNodes returns all registered nodes (for API).
func (r *Registry) ListNodes(ctx context.Context) ([]model.EdgeNode, error) {
	var nodes []model.EdgeNode
	err := r.db.WithContext(ctx).Order("region, node_id").Find(&nodes).Error
	return nodes, err
}

// NodeID returns this node's ID.
func (r *Registry) NodeID() string { return r.cfg.NodeID }

// Region returns this node's region.
func (r *Registry) Region() string { return r.cfg.Region }
