package model

import "time"

type ProxySetting struct {
	ID        uint   `gorm:"primaryKey"`
	Enabled   bool   `gorm:"not null;default:false"`
	Type      string `gorm:"size:16;not null;default:http"`
	Host      string `gorm:"size:255"`
	Port      int    `gorm:"not null;default:0"`
	Username  string `gorm:"size:255"`
	Password  string `gorm:"size:255"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
