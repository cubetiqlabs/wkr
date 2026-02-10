package server

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aspect-build/cubis-wkr/internal/config"
	"github.com/gofiber/fiber/v3"
)

func New(cfg config.ServerConfig, appCfg config.AppConfig) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      fmt.Sprintf("%s v%s", appCfg.Name, appCfg.Version),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		BodyLimit:    cfg.BodyLimit,
		ErrorHandler: errorHandler,
	})

	return app
}

func errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		msg = e.Message
	}

	slog.Error("request error", "status", code, "error", err.Error(), "path", c.Path())

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   msg,
	})
}

func Listen(app *fiber.App, cfg config.ServerConfig) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	slog.Info("starting cubis-wkr server", "address", addr)
	return app.Listen(addr, fiber.ListenConfig{
		EnablePrintRoutes: true,
	})
}

func Shutdown(app *fiber.App) error {
	slog.Info("shutting down server", "timeout", 10*time.Second)
	return app.ShutdownWithTimeout(10 * time.Second)
}
