package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/metrics"
	"github.com/cubetiqlabs/wkr/internal/model"
	"go.uber.org/zap"
)

// Router selects the best edge node for execution and handles failover.
type Router struct {
	registry *Registry
	client   *http.Client
}

func NewRouter(registry *Registry) *Router {
	return &Router{
		registry: registry,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SelectNode picks the best node for a given region preference.
// Strategy: least-loaded node in the preferred region, fallback to any region.
func (r *Router) SelectNode(preferredRegion string) *model.EdgeNode {
	nodes := r.registry.GetHealthyNodes(preferredRegion)
	if len(nodes) == 0 {
		// Fallback: any healthy node
		nodes = r.registry.GetHealthyNodes("")
	}
	if len(nodes) == 0 {
		return nil
	}

	// Sort by load (active/max ratio), pick least loaded
	sort.Slice(nodes, func(i, j int) bool {
		loadI := float64(nodes[i].ActiveWorkers) / float64(max(nodes[i].MaxWorkers, 1))
		loadJ := float64(nodes[j].ActiveWorkers) / float64(max(nodes[j].MaxWorkers, 1))
		return loadI < loadJ
	})

	return nodes[0]
}

// ForwardRequest proxies an invocation to a remote edge node with failover.
func (r *Router) ForwardRequest(ctx context.Context, node *model.EdgeNode, workerName string, method string, body []byte, headers map[string]string) (int, []byte, map[string]string, error) {
	url := fmt.Sprintf("%s/api/v1/invoke/%s", node.Endpoint, workerName)

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-Cubis-Forwarded-From", r.registry.NodeID())
	req.Header.Set("X-Cubis-Edge-Route", "true")

	start := time.Now()
	resp, err := r.client.Do(req)
	duration := time.Since(start)

	metrics.EdgeSyncDuration.WithLabelValues(r.registry.NodeID()).Observe(duration.Seconds())

	if err != nil {
		logger.Error("edge forward failed",
			zap.String("target_node", node.NodeID),
			zap.String("worker", workerName),
			zap.Error(err),
		)
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}
	respHeaders["X-Cubis-Edge-Node"] = node.NodeID
	respHeaders["X-Cubis-Edge-Region"] = node.Region

	return resp.StatusCode, respBody, respHeaders, nil
}

// ForwardWithFailover tries the primary node, then falls back to alternatives.
func (r *Router) ForwardWithFailover(ctx context.Context, workerName, method, preferredRegion string, body []byte, headers map[string]string) (int, []byte, map[string]string, error) {
	nodes := r.registry.GetHealthyNodes(preferredRegion)
	if len(nodes) == 0 {
		nodes = r.registry.GetHealthyNodes("")
	}

	// Sort by load
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ActiveWorkers < nodes[j].ActiveWorkers
	})

	var lastErr error
	for _, node := range nodes {
		if node.NodeID == r.registry.NodeID() {
			continue // don't forward to self
		}
		status, respBody, respHeaders, err := r.ForwardRequest(ctx, node, workerName, method, body, headers)
		if err != nil {
			lastErr = err
			metrics.EdgeFailovers.WithLabelValues(node.NodeID, r.registry.NodeID(), node.Region).Inc()
			continue
		}
		return status, respBody, respHeaders, nil
	}

	return 0, nil, nil, fmt.Errorf("all edge nodes failed: %w", lastErr)
}

// EdgeStatus returns cluster status for the health endpoint.
func (r *Router) EdgeStatus() map[string]interface{} {
	nodes := r.registry.GetHealthyNodes("")
	regions := make(map[string]int)
	totalActive := 0
	for _, n := range nodes {
		regions[n.Region]++
		totalActive += n.ActiveWorkers
	}
	return map[string]interface{}{
		"total_nodes":    len(nodes),
		"regions":        regions,
		"active_workers": totalActive,
		"this_node":      r.registry.NodeID(),
		"this_region":    r.registry.Region(),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SyncWorkerToEdge pushes a worker deployment to an edge node.
func (r *Router) SyncWorkerToEdge(ctx context.Context, node *model.EdgeNode, worker interface{}) error {
	url := fmt.Sprintf("%s/internal/sync/worker", node.Endpoint)
	data, _ := json.Marshal(worker)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cubis-Internal", "true")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync failed: %s", string(body))
	}
	return nil
}
