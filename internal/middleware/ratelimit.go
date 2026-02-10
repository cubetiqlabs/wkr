package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// RateLimiter implements a simple in-memory token bucket rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int
	window   time.Duration
}

type visitor struct {
	tokens    int
	lastReset time.Time
}

func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	// Cleanup stale entries periodically
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Handler() fiber.Handler {
	return func(c fiber.Ctx) error {
		ip := c.IP()

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		if !exists {
			rl.visitors[ip] = &visitor{tokens: rl.rate - 1, lastReset: time.Now()}
			rl.mu.Unlock()
			return c.Next()
		}

		if time.Since(v.lastReset) > rl.window {
			v.tokens = rl.rate - 1
			v.lastReset = time.Now()
			rl.mu.Unlock()
			return c.Next()
		}

		if v.tokens <= 0 {
			rl.mu.Unlock()
			c.Set("Retry-After", rl.window.String())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}

		v.tokens--
		rl.mu.Unlock()
		return c.Next()
	}
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastReset) > rl.window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}
