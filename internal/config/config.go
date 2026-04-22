package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	LegacyAuthKey                string `json:"auth-key"`
	Host                         string `json:"host"`
	Port                         int    `json:"port"`
	APIKey                       string `json:"api-key"`
	SessionSecret                string `json:"session-secret"`
	DatabaseDSN                  string `json:"database-dsn"`
	RefreshAccountIntervalMinute int    `json:"refresh-account-interval-minute"`
	TLSVerify                    bool   `json:"tls-verify"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Host:                         "0.0.0.0",
		Port:                         8080,
		APIKey:                       "chatgpt2api",
		SessionSecret:                "web2api-session-secret",
		DatabaseDSN:                  filepath.ToSlash(filepath.Join("data", "web2api.db")),
		RefreshAccountIntervalMinute: 30,
		TLSVerify:                    true,
	}

	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	overrideString(&cfg.Host, os.Getenv("WEB2API_HOST"))
	overrideInt(&cfg.Port, os.Getenv("WEB2API_PORT"))
	overrideString(&cfg.APIKey, os.Getenv("WEB2API_API_KEY"))
	overrideString(&cfg.SessionSecret, os.Getenv("WEB2API_SESSION_SECRET"))
	overrideString(&cfg.LegacyAuthKey, os.Getenv("WEB2API_AUTH_KEY"))
	overrideString(&cfg.DatabaseDSN, os.Getenv("WEB2API_DATABASE_DSN"))
	overrideInt(&cfg.RefreshAccountIntervalMinute, os.Getenv("WEB2API_REFRESH_ACCOUNT_INTERVAL_MINUTE"))
	overrideBool(&cfg.TLSVerify, os.Getenv("WEB2API_TLS_VERIFY"))

	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.SessionSecret = strings.TrimSpace(cfg.SessionSecret)
	cfg.LegacyAuthKey = strings.TrimSpace(cfg.LegacyAuthKey)
	cfg.DatabaseDSN = strings.TrimSpace(cfg.DatabaseDSN)

	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if cfg.APIKey == "" {
		cfg.APIKey = cfg.LegacyAuthKey
	}
	if cfg.APIKey == "" {
		return nil, errors.New("api-key is required")
	}
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = cfg.APIKey
	}
	if cfg.DatabaseDSN == "" {
		cfg.DatabaseDSN = filepath.ToSlash(filepath.Join("data", "web2api.db"))
	}
	if cfg.RefreshAccountIntervalMinute <= 0 {
		cfg.RefreshAccountIntervalMinute = 30
	}

	return cfg, nil
}

func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

func overrideString(dst *string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		*dst = value
	}
}

func overrideInt(dst *int, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		*dst = parsed
	}
}

func overrideBool(dst *bool, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		*dst = parsed
	}
}
