package handler

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// Response is a standard API response envelope.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type Meta struct {
	Page     int   `json:"page,omitempty"`
	PageSize int   `json:"page_size,omitempty"`
	Total    int64 `json:"total,omitempty"`
}

func ok(c fiber.Ctx, data interface{}) error {
	return c.JSON(Response{Success: true, Data: data})
}

func okWithMeta(c fiber.Ctx, data interface{}, meta *Meta) error {
	return c.JSON(Response{Success: true, Data: data, Meta: meta})
}

func created(c fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(Response{Success: true, Data: data})
}

func errResponse(c fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(Response{Success: false, Error: msg})
}

// clientIP extracts the real client IP from proxy headers, falling back to c.IP().
// Priority: CF-Connecting-IP (Cloudflare) → X-Real-IP (nginx) → X-Forwarded-For (first IP) → c.IP()
func clientIP(c fiber.Ctx) string {
	if ip := c.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := c.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := c.Get("X-Forwarded-For"); fwd != "" {
		if ip, _, _ := strings.Cut(fwd, ","); ip != "" {
			return strings.TrimSpace(ip)
		}
	}
	return c.IP()
}
