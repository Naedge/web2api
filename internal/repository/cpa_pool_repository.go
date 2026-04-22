package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"web2api/internal/model"
)

type CPAPoolRepository struct {
	db *gorm.DB
}

func NewCPAPoolRepository(db *gorm.DB) *CPAPoolRepository {
	return &CPAPoolRepository{db: db}
}

func (r *CPAPoolRepository) List(ctx context.Context) ([]model.CPAPool, error) {
	items := []model.CPAPool{}
	err := r.db.WithContext(ctx).Order("id asc").Find(&items).Error
	return items, err
}

func (r *CPAPoolRepository) GetByPoolID(ctx context.Context, poolID string) (*model.CPAPool, error) {
	var item model.CPAPool
	err := r.db.WithContext(ctx).Where("pool_id = ?", poolID).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *CPAPoolRepository) Create(ctx context.Context, item *model.CPAPool) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *CPAPoolRepository) Save(ctx context.Context, item *model.CPAPool) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *CPAPoolRepository) DeleteByPoolID(ctx context.Context, poolID string) (int64, error) {
	result := r.db.WithContext(ctx).Where("pool_id = ?", poolID).Delete(&model.CPAPool{})
	return result.RowsAffected, result.Error
}
