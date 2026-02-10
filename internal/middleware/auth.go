package middleware

import (
	"log/slog"
	"strings"

	"github.com/aspect-build/cubis-wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	RoleKey   contextKey = "role"
)

// Auth validates JWT tokens from the Authorization header.
func Auth(jwtSecret []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authorization header",
			})
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization format, use: Bearer <token>",
			})
		}

		claims, err := service.ValidateJWT(token, jwtSecret)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		userID, err := uuid.Parse(claims.Sub)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid token claims",
			})
		}

		c.Locals(string(UserIDKey), userID)
		c.Locals(string(RoleKey), string(claims.Role))
		return c.Next()
	}
}

// RequireAdmin ensures the authenticated user has admin role.
func RequireAdmin() fiber.Handler {
	return func(c fiber.Ctx) error {
		role, _ := c.Locals(string(RoleKey)).(string)
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "admin access required",
			})
		}
		return c.Next()
	}
}

// GetUserID extracts the authenticated user ID from context.
func GetUserID(c fiber.Ctx) (uuid.UUID, error) {
	id, ok := c.Locals(string(UserIDKey)).(uuid.UUID)
	if !ok {
		return uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "user not authenticated")
	}
	return id, nil
}

// RequestLogger logs each request.
func RequestLogger() fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		slog.Info("request",
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"ip", c.IP(),
		)
		return err
	}
}

// SecurityHeaders adds security headers to all responses.
func SecurityHeaders() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Set("Content-Security-Policy", "default-src 'none'")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		return c.Next()
	}
}
