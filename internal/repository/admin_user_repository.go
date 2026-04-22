package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"web2api/internal/model"
)

type AdminUserRepository struct {
	db *gorm.DB
}

func NewAdminUserRepository(db *gorm.DB) *AdminUserRepository {
	return &AdminUserRepository{db: db}
}

func (r *AdminUserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AdminUser{}).Count(&count).Error
	return count, err
}

func (r *AdminUserRepository) GetByID(ctx context.Context, id uint) (*model.AdminUser, error) {
	var item model.AdminUser
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AdminUserRepository) GetByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	var item model.AdminUser
	err := r.db.WithContext(ctx).Where("username = ?", username).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AdminUserRepository) GetByAPIKey(ctx context.Context, apiKey string) (*model.AdminUser, error) {
	var item model.AdminUser
	err := r.db.WithContext(ctx).Where("api_key = ?", apiKey).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AdminUserRepository) GetFirst(ctx context.Context) (*model.AdminUser, error) {
	var item model.AdminUser
	err := r.db.WithContext(ctx).Order("id asc").Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AdminUserRepository) Create(ctx context.Context, item *model.AdminUser) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *AdminUserRepository) Save(ctx context.Context, item *model.AdminUser) error {
	return r.db.WithContext(ctx).Save(item).Error
}
