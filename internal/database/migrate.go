package database

import (
	"log/slog"

	"github.com/aspect-build/cubis-wkr/internal/model"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	slog.Info("running database auto-migration")
	return db.AutoMigrate(
		&model.User{},
		&model.APIKey{},
		&model.Worker{},
		&model.Deployment{},
		&model.Invocation{},
	)
}
