package model

import "time"

const (
	PermVideoUpload   = "video:upload"
	PermVideoRead     = "video:read"
	PermVideoDownload = "video:download"
	PermVideoManage   = "video:manage"
	PermRBACManage    = "rbac:manage"

	RoleAdmin = "admin"
	RoleUser  = "user"

	UserStatusActive = 1

	UploadStatusUploading = "uploading"
	UploadStatusMerging   = "merging"
	UploadStatusCompleted = "completed"
	UploadStatusAborted   = "aborted"

	VideoStatusUploading   = "uploading"
	VideoStatusUploaded    = "uploaded"
	VideoStatusTranscoding = "transcoding"
	VideoStatusReady       = "ready"
	VideoStatusFailed      = "failed"

	RenditionPending = "pending"
	RenditionReady   = "ready"
	RenditionFailed  = "failed"
	RenditionSkipped = "skipped"
)

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Status       int       `gorm:"not null;default:1" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Roles        []Role    `gorm:"many2many:user_roles" json:"roles,omitempty"`
}

type Role struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:64;not null" json:"name"`
	Code        string       `gorm:"uniqueIndex;size:64;not null" json:"code"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Permissions []Permission `gorm:"many2many:role_permissions" json:"permissions,omitempty"`
}

type Permission struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"uniqueIndex;size:64;not null" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PasswordReset struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	TokenHash string    `gorm:"size:64;not null;index" json:"-"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	Used      bool      `gorm:"not null;default:false" json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

type UploadSession struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	UserID             uint      `gorm:"index;not null" json:"user_id"`
	OriginalFilename   string    `gorm:"size:255;not null" json:"original_filename"`
	StoredBasename     string    `gorm:"size:64;not null" json:"stored_basename"`
	Ext                string    `gorm:"size:16;not null" json:"ext"`
	TotalSize          int64     `gorm:"not null" json:"total_size"`
	ChunkSize          int64     `gorm:"not null" json:"chunk_size"`
	ChunkTotal         int       `gorm:"not null" json:"chunk_total"`
	UploadedChunksJSON string    `gorm:"type:text" json:"-"`
	UploadProgress     int       `gorm:"not null;default:0" json:"upload_progress"`
	Status             string    `gorm:"size:32;not null;index" json:"status"`
	VideoID            *uint     `json:"video_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Video struct {
	ID                uint             `gorm:"primaryKey" json:"id"`
	OwnerID           uint             `gorm:"index;not null" json:"owner_id"`
	OriginalFilename  string           `gorm:"size:255;not null" json:"original_filename"`
	StoredBasename    string           `gorm:"size:64;not null" json:"stored_basename"`
	SourcePath        string           `gorm:"size:512;not null" json:"source_path"`
	UploadProgress    int              `gorm:"not null;default:0" json:"upload_progress"`
	TranscodeProgress int              `gorm:"not null;default:0" json:"transcode_progress"`
	Status            string           `gorm:"size:32;not null;index" json:"status"`
	DurationMS        int64            `json:"duration_ms"`
	Width             int              `json:"width"`
	Height            int              `json:"height"`
	ViewCount         int64            `gorm:"not null;default:0" json:"view_count"`
	DownloadCount     int64            `gorm:"not null;default:0" json:"download_count"`
	ErrorMessage      string           `gorm:"size:512" json:"error_message,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	Renditions        []VideoRendition `json:"renditions,omitempty"`
}

type VideoRendition struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	VideoID   uint      `gorm:"index;not null" json:"video_id"`
	Quality   string    `gorm:"size:16;not null" json:"quality"`
	Path      string    `gorm:"size:512" json:"path"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Status    string    `gorm:"size:32;not null" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
