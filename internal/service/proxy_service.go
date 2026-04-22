package service

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"web2api/internal/dto"
	"web2api/internal/model"
	"web2api/internal/repository"
)

type ProxyConfig struct {
	Enabled  bool
	Type     string
	Host     string
	Port     int
	Username string
	Password string
}

type ProxyService struct {
	repo *repository.ProxySettingRepository
}

func NewProxyService(repo *repository.ProxySettingRepository) *ProxyService {
	return &ProxyService{repo: repo}
}

func (s *ProxyService) Get(ctx context.Context) (*dto.ProxySettingsResponse, error) {
	item, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return &dto.ProxySettingsResponse{
			Enabled: false,
			Type:    "http",
		}, nil
	}
	return proxyModelToResponse(item), nil
}

func (s *ProxyService) Save(
	ctx context.Context,
	req dto.ProxySettingsRequest,
) (*dto.ProxySettingsResponse, error) {
	normalized, err := normalizeProxyRequest(req)
	if err != nil {
		return nil, err
	}

	item, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if item == nil {
		item = &model.ProxySetting{}
	}

	item.Enabled = normalized.Enabled
	item.Type = normalized.Type
	item.Host = normalized.Host
	item.Port = normalized.Port
	item.Username = normalized.Username
	item.Password = normalized.Password

	if err := s.repo.Save(ctx, item); err != nil {
		return nil, err
	}
	return proxyModelToResponse(item), nil
}

func (s *ProxyService) ActiveConfig(ctx context.Context) (*ProxyConfig, error) {
	item, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if item == nil || !item.Enabled {
		return nil, nil
	}
	return &ProxyConfig{
		Enabled:  item.Enabled,
		Type:     item.Type,
		Host:     item.Host,
		Port:     item.Port,
		Username: item.Username,
		Password: item.Password,
	}, nil
}

func (c *ProxyConfig) URL() (*url.URL, error) {
	if c == nil || !c.Enabled {
		return nil, nil
	}
	u := &url.URL{
		Scheme: c.Type,
		Host:   c.Host + ":" + strconv.Itoa(c.Port),
	}
	if c.Username != "" {
		if c.Password != "" {
			u.User = url.UserPassword(c.Username, c.Password)
		} else {
			u.User = url.User(c.Username)
		}
	}
	return u, nil
}

func normalizeProxyRequest(req dto.ProxySettingsRequest) (*dto.ProxySettingsRequest, error) {
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Type == "" {
		req.Type = "http"
	}
	switch req.Type {
	case "http", "https", "socks5":
	default:
		return nil, badRequest("type must be http, https or socks5")
	}

	if req.Enabled {
		if req.Host == "" {
			return nil, badRequest("host is required")
		}
		if req.Port <= 0 || req.Port > 65535 {
			return nil, badRequest("port is invalid")
		}
	}

	return &req, nil
}

func proxyModelToResponse(item *model.ProxySetting) *dto.ProxySettingsResponse {
	return &dto.ProxySettingsResponse{
		Enabled:  item.Enabled,
		Type:     item.Type,
		Host:     item.Host,
		Port:     item.Port,
		Username: item.Username,
		Password: item.Password,
	}
}
