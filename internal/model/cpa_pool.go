package model

import "time"

type CPAPool struct {
	ID            uint   `gorm:"primaryKey"`
	PoolID        string `gorm:"size:64;uniqueIndex;not null"`
	Name          string `gorm:"size:255"`
	BaseURL       string `gorm:"size:1024;not null"`
	SecretKey     string `gorm:"size:1024;not null"`
	ImportJobJSON string `gorm:"type:text"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
