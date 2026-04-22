package storage

import (
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"web2api/internal/config"
	"web2api/internal/model"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	if isSQLiteDSN(cfg.DatabaseDSN) {
		dbPath := cfg.DatabaseDSN
		if dir := filepath.Dir(dbPath); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}

	db, err := gorm.Open(sqlite.Open(cfg.DatabaseDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&model.AdminUser{}, &model.Account{}, &model.CPAPool{}); err != nil {
		return nil, err
	}

	return db, nil
}

func isSQLiteDSN(dsn string) bool {
	dsn = strings.TrimSpace(strings.ToLower(dsn))
	return (dsn != "" &&
		!strings.Contains(dsn, "://") &&
		!strings.HasPrefix(dsn, "file:")) ||
		strings.HasSuffix(dsn, ".db") ||
		strings.HasSuffix(dsn, ".sqlite")
}
