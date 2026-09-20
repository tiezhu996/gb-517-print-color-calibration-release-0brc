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

	// 漂移门禁相关查询。
	ListRecentAccepted(ctx context.Context, relatedCode, category string, excludeID uint, limit int) ([]model.ColorProof, error)
	FindActiveNewerProof(ctx context.Context, relatedCode, category string, createdAt time.Time, id uint) (model.ColorProof, bool, error)
	ListByIDs(ctx context.Context, ids []uint) ([]model.ColorProof, error)
	UpdateGateFields(ctx context.Context, id uint, fields map[string]any) error
	ListByStatus(ctx context.Context, status string) ([]model.ColorProof, error)
	CountOpenBlocked(ctx context.Context) (int64, error)
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

// ListRecentAccepted 返回同一关联编码与类别下最近接受的校样（不含被排除记录），
// 按接受时间倒序，供漂移基准取中位数。
func (r *colorProofRepository) ListRecentAccepted(ctx context.Context, relatedCode, category string, excludeID uint, limit int) ([]model.ColorProof, error) {
	items := make([]model.ColorProof, 0, limit)
	err := r.db.WithContext(ctx).
		Where("related_code = ? AND category = ? AND status = ?", relatedCode, category, "accepted").
		Not("id = ?", excludeID).
		// id 为单调递增主键，直接反映接受先后；updated_at 作为次级次序。
		Order("id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// FindActiveNewerProof 判断是否存在同一关联编码与类别下更新的、仍有效（未拒绝、
// 未删除）的校样；存在即说明当前校样已被取代。
func (r *colorProofRepository) FindActiveNewerProof(ctx context.Context, relatedCode, category string, createdAt time.Time, id uint) (model.ColorProof, bool, error) {
	var newer model.ColorProof
	err := r.db.WithContext(ctx).
		Where("related_code = ? AND category = ?", relatedCode, category).
		Where("status <> ?", "rejected").
		Where("id <> ?", id).
		Where("created_at > ? OR (created_at = ? AND id > ?)", createdAt, createdAt, id).
		Order("created_at DESC, id DESC").
		First(&newer).Error
	if err == gorm.ErrRecordNotFound {
		return model.ColorProof{}, false, nil
	}
	if err != nil {
		return model.ColorProof{}, false, err
	}
	return newer, true, nil
}

func (r *colorProofRepository) ListByIDs(ctx context.Context, ids []uint) ([]model.ColorProof, error) {
	items := make([]model.ColorProof, 0, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&items).Error
	return items, err
}

// UpdateGateFields 只写入漂移门禁快照，不触碰 version，避免基准刷新与乐观锁互相干扰。
func (r *colorProofRepository) UpdateGateFields(ctx context.Context, id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.ColorProof{}).
		Where("id = ?", id).
		Updates(fields).Error
}

func (r *colorProofRepository) ListByStatus(ctx context.Context, status string) ([]model.ColorProof, error) {
	items := make([]model.ColorProof, 0)
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("id DESC").
		Find(&items).Error
	return items, err
}

// CountOpenBlocked 统计仍在 captured/review 待办状态、且漂移门禁阻断的校样数量。
func (r *colorProofRepository) CountOpenBlocked(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&model.ColorProof{}).
		Where("gate_status = ? AND status IN ?", "blocked", []string{"captured", "review"}).
		Count(&total).Error
	return total, err
}
