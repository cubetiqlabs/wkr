package edge

import (
	"bytes"
	"context"
	"crypto/subtle"
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
	registry       *Registry
	client         *http.Client
	internalSecret string
}

func NewRouter(registry *Registry, internalSecret string) *Router {
	return &Router{
		registry:       registry,
		internalSecret: internalSecret,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SelectNode picks the least-loaded healthy node for a given region preference.
func (r *Router) SelectNode(preferredRegion string) *model.EdgeNode {
	nodes := r.registry.GetHealthyNodes(preferredRegion)
	if len(nodes) == 0 && preferredRegion != "" {
		nodes = r.registry.GetHealthyNodes("")
	}
	if len(nodes) == 0 {
		return nil
	}

	sort.Slice(nodes, func(i, j int) bool {
		loadI := float64(nodes[i].ActiveWorkers) / float64(max(nodes[i].MaxWorkers, 1))
		loadJ := float64(nodes[j].ActiveWorkers) / float64(max(nodes[j].MaxWorkers, 1))
		return loadI < loadJ
	})

	return nodes[0]
}

// ShouldForwardToEdge returns a remote node if this request should be forwarded,
// or nil if it should be executed locally. Skips self and already-forwarded requests.
func (r *Router) ShouldForwardToEdge(isAlreadyForwarded bool) *model.EdgeNode {
	if isAlreadyForwarded {
		return nil // prevent forwarding loops
	}

	self := r.registry.NodeID()
	nodes := r.registry.GetHealthyNodes("")
	if len(nodes) <= 1 {
		return nil // only this node, execute locally
	}

	// Find least-loaded node; if it's us, execute locally
	sort.Slice(nodes, func(i, j int) bool {
		loadI := float64(nodes[i].ActiveWorkers) / float64(max(nodes[i].MaxWorkers, 1))
		loadJ := float64(nodes[j].ActiveWorkers) / float64(max(nodes[j].MaxWorkers, 1))
		return loadI < loadJ
	})

	best := nodes[0]
	if best.NodeID == self {
		return nil
	}
	return best
}

// ForwardRequest proxies an invocation to a remote edge node.
func (r *Router) ForwardRequest(ctx context.Context, node *model.EdgeNode, workerName, method string, body []byte, headers map[string]string) (int, []byte, map[string]string, error) {
	url := fmt.Sprintf("%s/api/v1/invoke/%s", node.Endpoint, workerName)

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-Cubis-Forwarded-From", r.registry.NodeID())
	req.Header.Set("X-Cubis-Edge-Route", "true") // loop prevention flag

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

// ForwardWithFailover tries nodes sequentially until one succeeds.
func (r *Router) ForwardWithFailover(ctx context.Context, workerName, method, preferredRegion string, body []byte, headers map[string]string) (int, []byte, map[string]string, error) {
	nodes := r.registry.GetHealthyNodes(preferredRegion)
	if len(nodes) == 0 && preferredRegion != "" {
		nodes = r.registry.GetHealthyNodes("")
	}
	if len(nodes) == 0 {
		return 0, nil, nil, fmt.Errorf("no healthy edge nodes available")
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ActiveWorkers < nodes[j].ActiveWorkers
	})

	var lastErr error
	for _, node := range nodes {
		if node.NodeID == r.registry.NodeID() {
			continue
		}
		status, respBody, respHeaders, err := r.ForwardRequest(ctx, node, workerName, method, body, headers)
		if err != nil {
			lastErr = err
			metrics.EdgeFailovers.WithLabelValues(node.NodeID, r.registry.NodeID(), node.Region).Inc()
			continue
		}
		return status, respBody, respHeaders, nil
	}

	if lastErr == nil {
		return 0, nil, nil, fmt.Errorf("no remote edge nodes to forward to")
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

// SyncWorkerToEdge pushes a worker deployment to an edge node.
func (r *Router) SyncWorkerToEdge(ctx context.Context, node *model.EdgeNode, worker interface{}) error {
	url := fmt.Sprintf("%s/internal/sync/worker", node.Endpoint)
	data, err := json.Marshal(worker)
	if err != nil {
		return fmt.Errorf("marshal worker: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.internalSecret != "" {
		req.Header.Set("X-Cubis-Internal-Secret", r.internalSecret)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// ValidateInternalSecret checks the shared secret using constant-time comparison.
func (r *Router) ValidateInternalSecret(secret string) bool {
	if r.internalSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.internalSecret), []byte(secret)) == 1
}
