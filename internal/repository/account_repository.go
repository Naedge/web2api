package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"web2api/internal/model"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) List(ctx context.Context) ([]model.Account, error) {
	items := []model.Account{}
	err := r.db.WithContext(ctx).Order("id asc").Find(&items).Error
	return items, err
}

func (r *AccountRepository) ListByTokens(ctx context.Context, tokens []string) ([]model.Account, error) {
	items := []model.Account{}
	if len(tokens) == 0 {
		return items, nil
	}
	err := r.db.WithContext(ctx).Where("access_token IN ?", tokens).Order("id asc").Find(&items).Error
	return items, err
}

func (r *AccountRepository) ListLimited(ctx context.Context) ([]model.Account, error) {
	items := []model.Account{}
	err := r.db.WithContext(ctx).
		Where("status = ?", model.AccountStatusLimited).
		Order("id asc").
		Find(&items).Error
	return items, err
}

func (r *AccountRepository) ListAvailableCandidates(ctx context.Context) ([]model.Account, error) {
	items := []model.Account{}
	err := r.db.WithContext(ctx).
		Where("status = ? AND quota > 0", model.AccountStatusNormal).
		Order("id asc").
		Find(&items).Error
	return items, err
}

func (r *AccountRepository) GetByToken(ctx context.Context, token string) (*model.Account, error) {
	var item model.Account
	err := r.db.WithContext(ctx).Where("access_token = ?", token).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AccountRepository) Create(ctx context.Context, item *model.Account) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *AccountRepository) Save(ctx context.Context, item *model.Account) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *AccountRepository) UpdateByToken(ctx context.Context, token string, values map[string]any) error {
	return r.db.WithContext(ctx).
		Model(&model.Account{}).
		Where("access_token = ?", token).
		Updates(values).Error
}

func (r *AccountRepository) DeleteByTokens(ctx context.Context, tokens []string) (int64, error) {
	if len(tokens) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("access_token IN ?", tokens).Delete(&model.Account{})
	return result.RowsAffected, result.Error
}
