package database

import (
	"github.com/cubetiqlabs/cubis-wkr/internal/logger"
	"github.com/cubetiqlabs/cubis-wkr/internal/model"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	logger.Info("running database auto-migration")
	return db.AutoMigrate(
		&model.User{},
		&model.APIKey{},
		&model.Worker{},
		&model.Deployment{},
		&model.Invocation{},
	)
}
