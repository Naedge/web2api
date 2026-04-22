package model

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"time"
)

const (
	AccountStatusNormal  = "正常"
	AccountStatusLimited = "限流"
	AccountStatusInvalid = "异常"
)

type Account struct {
	ID               uint   `gorm:"primaryKey"`
	AccessToken      string `gorm:"size:4096;uniqueIndex;not null"`
	Type             string `gorm:"size:64;not null;default:Free"`
	Status           string `gorm:"size:64;not null;default:正常"`
	Quota            int    `gorm:"not null;default:0"`
	Email            string `gorm:"size:255"`
	UserID           string `gorm:"size:255"`
	LimitsProgress   string `gorm:"type:text"`
	DefaultModelSlug string `gorm:"size:128"`
	RestoreAt        string `gorm:"size:128"`
	Success          int    `gorm:"not null;default:0"`
	Fail             int    `gorm:"not null;default:0"`
	LastUsedAt       *time.Time
	UserAgent        string `gorm:"size:1024"`
	Impersonate      string `gorm:"size:128"`
	OAIDeviceID      string `gorm:"size:128"`
	OAISessionID     string `gorm:"size:128"`
	SecCHUA          string `gorm:"size:1024"`
	SecCHUAMobile    string `gorm:"size:64"`
	SecCHUAPlatform  string `gorm:"size:128"`
	Fingerprint      string `gorm:"type:text"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (a Account) PublicID() string {
	sum := sha1.Sum([]byte(a.AccessToken))
	return hex.EncodeToString(sum[:])[:16]
}

func (a Account) LimitsProgressValue() []map[string]any {
	if a.LimitsProgress == "" {
		return []map[string]any{}
	}

	items := []map[string]any{}
	if err := json.Unmarshal([]byte(a.LimitsProgress), &items); err != nil {
		return []map[string]any{}
	}

	return items
}
