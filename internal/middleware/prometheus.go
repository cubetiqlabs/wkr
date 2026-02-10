package middleware

import (
	"strconv"
	"time"

	m "github.com/cubetiqlabs/wkr/internal/metrics"
	"github.com/gofiber/fiber/v3"
)

// PrometheusMiddleware records HTTP request metrics.
func PrometheusMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		status := strconv.Itoa(c.Response().StatusCode())
		method := c.Method()
		path := c.Route().Path // use route pattern, not actual path (avoids cardinality explosion)

		m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())

		return err
	}
}
