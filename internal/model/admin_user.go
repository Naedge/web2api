package model

import "time"

type AdminUser struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"size:128;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
