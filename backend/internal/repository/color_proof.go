package repository

import (
	"context"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"gorm.io/gorm"
)

// ColorProofRepository owns all persistence operations for 色彩校样.
type ColorProofRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.ColorProof], error)
	Get(context.Context, uint) (model.ColorProof, error)
	Create(context.Context, *model.ColorProof) error
	Update(context.Context, uint, uint, *model.ColorProof) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	RecentAccepted(ctx context.Context, relatedCode, category string, excludeID uint, limit int) ([]model.ColorProof, error)
	HasNewerActiveProof(ctx context.Context, relatedCode string, effectiveAt time.Time, excludeID uint) (bool, error)
	ListAll(context.Context) ([]model.ColorProof, error)
}

type colorProofRepository struct {
	store *Store[model.ColorProof]
	db    *gorm.DB
}

func NewColorProofRepository(db *gorm.DB) ColorProofRepository {
	return &colorProofRepository{store: NewStore[model.ColorProof](db), db: db}
}

func (r *colorProofRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.ColorProof], error) {
	return r.store.List(ctx, q)
}
func (r *colorProofRepository) Get(ctx context.Context, id uint) (model.ColorProof, error) {
	return r.store.Get(ctx, id)
}
func (r *colorProofRepository) Create(ctx context.Context, item *model.ColorProof) error {
	return r.store.Create(ctx, item)
}
func (r *colorProofRepository) Update(ctx context.Context, id, version uint, item *model.ColorProof) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *colorProofRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *colorProofRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

// RecentAccepted returns up to limit most recently accepted proofs matching the
// same related code and category, ordered newest first. Proofs with an empty
// related code are only grouped by category within the same empty code.
func (r *colorProofRepository) RecentAccepted(ctx context.Context, relatedCode, category string, excludeID uint, limit int) ([]model.ColorProof, error) {
	if limit < 1 {
		limit = model.ColorProofDriftSampleLimit
	}
	items := make([]model.ColorProof, 0, limit)
	err := r.db.WithContext(ctx).
		Where("related_code = ? AND category = ? AND status = ? AND id <> ?", relatedCode, category, "accepted", excludeID).
		Order("effective_at DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// HasNewerActiveProof reports whether another non-rejected proof with the same
// related code was effective after the given proof, meaning the proof has been
// superseded by a newer校样 and can no longer be accepted.
func (r *colorProofRepository) HasNewerActiveProof(ctx context.Context, relatedCode string, effectiveAt time.Time, excludeID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ColorProof{}).
		Where("related_code = ? AND id <> ? AND status <> ?", relatedCode, excludeID, "rejected").
		Where("effective_at > ? OR (effective_at = ? AND id > ?)", effectiveAt, effectiveAt, excludeID).
		Count(&count).Error
	return count > 0, err
}

// ListAll returns every non-deleted proof for the calibration summary refresh.
func (r *colorProofRepository) ListAll(ctx context.Context) ([]model.ColorProof, error) {
	items := make([]model.ColorProof, 0)
	err := r.db.WithContext(ctx).Order("related_code ASC, category ASC, effective_at DESC, id DESC").Find(&items).Error
	return items, err
}
