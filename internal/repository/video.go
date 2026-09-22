package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"videoview/internal/model"
)

type ResetRepo struct {
	db *gorm.DB
}

func NewResetRepo(db *gorm.DB) *ResetRepo {
	return &ResetRepo{db: db}
}

func (r *ResetRepo) Create(ctx context.Context, rec *model.PasswordReset) error {
	return r.db.WithContext(ctx).Create(rec).Error
}

func (r *ResetRepo) FindValid(ctx context.Context, tokenHash string, now time.Time) (*model.PasswordReset, error) {
	var rec model.PasswordReset
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND used = ? AND expires_at > ?", tokenHash, false, now).
		First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *ResetRepo) MarkUsed(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.PasswordReset{}).Where("id = ?", id).Update("used", true).Error
}

type UploadRepo struct {
	db *gorm.DB
}

func NewUploadRepo(db *gorm.DB) *UploadRepo {
	return &UploadRepo{db: db}
}

func (r *UploadRepo) Create(ctx context.Context, s *model.UploadSession) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *UploadRepo) FindByID(ctx context.Context, id uint) (*model.UploadSession, error) {
	var s model.UploadSession
	err := r.db.WithContext(ctx).First(&s, id).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *UploadRepo) Save(ctx context.Context, s *model.UploadSession) error {
	return r.db.WithContext(ctx).Save(s).Error
}

type VideoRepo struct {
	db *gorm.DB
}

func NewVideoRepo(db *gorm.DB) *VideoRepo {
	return &VideoRepo{db: db}
}

func (r *VideoRepo) Create(ctx context.Context, v *model.Video) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *VideoRepo) Save(ctx context.Context, v *model.Video) error {
	return r.db.WithContext(ctx).Save(v).Error
}

func (r *VideoRepo) FindByID(ctx context.Context, id uint) (*model.Video, error) {
	var v model.Video
	err := r.db.WithContext(ctx).Preload("Renditions").First(&v, id).Error
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *VideoRepo) List(ctx context.Context, ownerID *uint) ([]model.Video, error) {
	q := r.db.WithContext(ctx).Preload("Renditions").Order("id DESC")
	if ownerID != nil {
		q = q.Where("owner_id = ?", *ownerID)
	}
	var list []model.Video
	err := q.Find(&list).Error
	return list, err
}

func (r *VideoRepo) UpdateProgress(ctx context.Context, id uint, uploadProgress, transcodeProgress int, status string) error {
	updates := map[string]any{}
	if uploadProgress >= 0 {
		updates["upload_progress"] = uploadProgress
	}
	if transcodeProgress >= 0 {
		updates["transcode_progress"] = transcodeProgress
	}
	if status != "" {
		updates["status"] = status
	}
	return r.db.WithContext(ctx).Model(&model.Video{}).Where("id = ?", id).Updates(updates).Error
}

func (r *VideoRepo) ClaimUploaded(ctx context.Context, id uint) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Video{}).
		Where("id = ? AND status = ?", id, model.VideoStatusUploaded).
		Updates(map[string]any{
			"status":             model.VideoStatusTranscoding,
			"transcode_progress": 0,
		})
	return res.RowsAffected > 0, res.Error
}

func (r *VideoRepo) ListUploadedIDs(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).Model(&model.Video{}).
		Where("status = ?", model.VideoStatusUploaded).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *VideoRepo) IncrementView(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.Video{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

func (r *VideoRepo) IncrementDownload(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.Video{}).
		Where("id = ?", id).
		UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error
}

func (r *VideoRepo) ReplaceRenditions(ctx context.Context, videoID uint, items []model.VideoRendition) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("video_id = ?", videoID).Delete(&model.VideoRendition{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
}

func (r *VideoRepo) SaveRendition(ctx context.Context, item *model.VideoRendition) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *VideoRepo) MarkFailed(ctx context.Context, id uint, msg string) error {
	return r.db.WithContext(ctx).Model(&model.Video{}).Where("id = ?", id).Updates(map[string]any{
		"status":        model.VideoStatusFailed,
		"error_message": msg,
	}).Error
}

func (r *VideoRepo) MarkReady(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.Video{}).Where("id = ?", id).Updates(map[string]any{
		"status":             model.VideoStatusReady,
		"transcode_progress": 100,
		"error_message":      "",
	}).Error
}
