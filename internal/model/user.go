package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a platform user.
type User struct {
	Base
	Email    string `gorm:"uniqueIndex;size:255;not null" json:"email" validate:"required,email"`
	Username string `gorm:"uniqueIndex;size:100;not null" json:"username"`
	Name     string `gorm:"size:255;not null" json:"name" validate:"required,min=2,max=255"`
	Password string `gorm:"size:255;not null" json:"-" validate:"required,min=8"`
	Role     Role   `gorm:"size:20;not null;default:'user'" json:"role"`
	Active   bool   `gorm:"not null;default:true" json:"active"`
}

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// APIKey provides programmatic access.
type APIKey struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	User      *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Name      string     `gorm:"size:255;not null" json:"name"`
	KeyHash   string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	Prefix    string     `gorm:"size:10;not null" json:"prefix"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (a *APIKey) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.Must(uuid.NewV7())
	}
	return nil
}
