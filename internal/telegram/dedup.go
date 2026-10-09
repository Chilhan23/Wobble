package telegram

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type DedupStore interface {
	RecordUpdateID(ctx context.Context, updateID int64) (isNew bool, err error)
}

type dedupStore struct {
	db *gorm.DB
}

func NewDedupStore(db *gorm.DB) DedupStore {
	return &dedupStore{db: db}
}

func (s *dedupStore) RecordUpdateID(ctx context.Context, updateID int64) (bool, error) {
	res := s.db.WithContext(ctx).Exec(`
		INSERT INTO telegram_updates (update_id)
		VALUES (?)
		ON CONFLICT (update_id) DO NOTHING
	`, updateID)

	if res.Error != nil {
		return false, fmt.Errorf("failed to record telegram update id: %w", res.Error)
	}

	return res.RowsAffected > 0, nil
}
