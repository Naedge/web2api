package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"web2api/internal/model"
)

type ProxySettingRepository struct {
	db *gorm.DB
}

func NewProxySettingRepository(db *gorm.DB) *ProxySettingRepository {
	return &ProxySettingRepository{db: db}
}

func (r *ProxySettingRepository) Get(ctx context.Context) (*model.ProxySetting, error) {
	var item model.ProxySetting
	err := r.db.WithContext(ctx).Order("id asc").Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *ProxySettingRepository) Save(ctx context.Context, item *model.ProxySetting) error {
	return r.db.WithContext(ctx).Save(item).Error
}
