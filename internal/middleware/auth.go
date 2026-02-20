package middleware

import (
	"strings"

	"github.com/cubetiqlabs/wkr/internal/logger"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	RoleKey   contextKey = "role"
)

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

func GetUserID(c fiber.Ctx) (uuid.UUID, error) {
	id, ok := c.Locals(string(UserIDKey)).(uuid.UUID)
	if !ok {
		return uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "user not authenticated")
	}
	return id, nil
}

// ParseUserIDFromToken validates a raw JWT and returns the user ID.
// Used for WebSocket auth where the token comes from a query param.
func ParseUserIDFromToken(token string, jwtSecret []byte) (uuid.UUID, error) {
	claims, err := service.ValidateJWT(token, jwtSecret)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(claims.Sub)
}

func RequestLogger() fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		logger.Info("request",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", c.Response().StatusCode()),
			zap.String("ip", clientIP(c)),
		)
		return err
	}
}

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
