package tenant

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("tenant not found")
	ErrConflict = errors.New("tenant already exists")
)

type Repository interface {
	GetByAPIKeyHash(ctx context.Context, hash string) (*Tenant, error)
	GetByID(ctx context.Context, id int64) (*Tenant, error)
	GetByKeyIdentifier(ctx context.Context, keyIdentifier string) (*Tenant, error)
	Create(ctx context.Context, t *Tenant) error
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) GetByAPIKeyHash(ctx context.Context, hash string) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).Where("api_key_hash = ?", hash).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant by api key hash: %w", err)
	}
	return &t, nil
}

func (r *repository) GetByID(ctx context.Context, id int64) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant by id: %w", err)
	}
	return &t, nil
}

func (r *repository) GetByKeyIdentifier(ctx context.Context, keyIdentifier string) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).Where("key_identifier = ?", keyIdentifier).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant by key identifier: %w", err)
	}
	return &t, nil
}

func (r *repository) Create(ctx context.Context, t *Tenant) error {
	err := r.db.WithContext(ctx).Create(t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrConflict
		}
		return fmt.Errorf("failed to create tenant: %w", err)
	}
	return nil
}
