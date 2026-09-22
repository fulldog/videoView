package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"videoview/internal/model"
	"videoview/internal/repository"
	"videoview/internal/transcode"
	"videoview/internal/upload"
)

var (
	ErrBadExt         = errors.New("unsupported video format")
	ErrSessionGone    = errors.New("upload session not found")
	ErrForbiddenOwner = errors.New("not upload owner")
	ErrBadChunk       = errors.New("invalid chunk index")
	ErrIncomplete     = errors.New("chunks incomplete")
	ErrSessionDone    = errors.New("upload already completed")
	ErrVideoGone      = errors.New("video not found")
	ErrNotReady       = errors.New("video not ready")
	ErrNoRendition    = errors.New("requested quality not available")
)

var allowedExt = map[string]struct{}{
	"mp4": {}, "mov": {}, "mkv": {}, "webm": {}, "avi": {},
	"flv": {}, "m4v": {}, "mpeg": {}, "mpg": {}, "wmv": {}, "ts": {},
}

type UploadService struct {
	sessions *repository.UploadRepo
	videos   *repository.VideoRepo
	store    *upload.Store
	worker   *transcode.Worker
}

func NewUploadService(sessions *repository.UploadRepo, videos *repository.VideoRepo, store *upload.Store, worker *transcode.Worker) *UploadService {
	return &UploadService{sessions: sessions, videos: videos, store: store, worker: worker}
}

func (s *UploadService) Init(ctx context.Context, userID uint, filename string, size, chunkSize int64) (*model.UploadSession, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if _, ok := allowedExt[ext]; !ok {
		return nil, ErrBadExt
	}
	if size <= 0 {
		return nil, fmt.Errorf("invalid size")
	}
	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024
	}
	total := int((size + chunkSize - 1) / chunkSize)
	sess := &model.UploadSession{
		UserID:             userID,
		OriginalFilename:   filepath.Base(filename),
		StoredBasename:     strings.ReplaceAll(uuid.NewString(), "-", ""),
		Ext:                ext,
		TotalSize:          size,
		ChunkSize:          chunkSize,
		ChunkTotal:         total,
		UploadedChunksJSON: "[]",
		UploadProgress:     0,
		Status:             model.UploadStatusUploading,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *UploadService) PutChunk(ctx context.Context, userID, sessionID uint, index int, body io.Reader) (*model.UploadSession, error) {
	sess, err := s.mustOwnSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if sess.Status != model.UploadStatusUploading {
		return nil, ErrSessionDone
	}
	if index < 0 || index >= sess.ChunkTotal {
		return nil, ErrBadChunk
	}
	if _, err := s.store.WriteChunk(sessionID, index, body); err != nil {
		return nil, err
	}
	idxs, err := s.store.ListChunks(sessionID)
	if err != nil {
		return nil, err
	}
	sess.UploadProgress = len(idxs) * 100 / sess.ChunkTotal
	b, _ := json.Marshal(idxs)
	sess.UploadedChunksJSON = string(b)
	if err := s.sessions.Save(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *UploadService) Get(ctx context.Context, userID, sessionID uint) (*model.UploadSession, []int, error) {
	sess, err := s.mustOwnSession(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	idxs, err := s.store.ListChunks(sessionID)
	if err != nil {
		return nil, nil, err
	}
	sort.Ints(idxs)
	return sess, idxs, nil
}

func (s *UploadService) Complete(ctx context.Context, userID, sessionID uint) (*model.Video, error) {
	sess, err := s.mustOwnSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if sess.Status == model.UploadStatusCompleted && sess.VideoID != nil {
		return s.videos.FindByID(ctx, *sess.VideoID)
	}
	idxs, err := s.store.ListChunks(sessionID)
	if err != nil {
		return nil, err
	}
	if len(idxs) != sess.ChunkTotal {
		return nil, ErrIncomplete
	}
	have := map[int]struct{}{}
	for _, i := range idxs {
		have[i] = struct{}{}
	}
	for i := 0; i < sess.ChunkTotal; i++ {
		if _, ok := have[i]; !ok {
			return nil, ErrIncomplete
		}
	}
	sess.Status = model.UploadStatusMerging
	if err := s.sessions.Save(ctx, sess); err != nil {
		return nil, err
	}
	dest := s.store.OriginalPath(sess.StoredBasename, sess.Ext)
	if err := s.store.Merge(sessionID, dest, sess.ChunkTotal); err != nil {
		sess.Status = model.UploadStatusUploading
		_ = s.sessions.Save(ctx, sess)
		return nil, err
	}
	info, err := s.worker.Probe(ctx, dest)
	if err != nil {
		sess.Status = model.UploadStatusUploading
		_ = s.sessions.Save(ctx, sess)
		return nil, fmt.Errorf("probe source: %w", err)
	}
	v := &model.Video{
		OwnerID:           userID,
		OriginalFilename:  sess.OriginalFilename,
		StoredBasename:    sess.StoredBasename,
		SourcePath:        dest,
		UploadProgress:    100,
		TranscodeProgress: 0,
		Status:            model.VideoStatusUploaded,
		DurationMS:        info.Duration.Milliseconds(),
		Width:             info.Width,
		Height:            info.Height,
	}
	if err := s.videos.Create(ctx, v); err != nil {
		return nil, err
	}
	sess.Status = model.UploadStatusCompleted
	sess.UploadProgress = 100
	sess.VideoID = &v.ID
	if err := s.sessions.Save(ctx, sess); err != nil {
		return nil, err
	}
	_ = s.store.RemoveChunks(sessionID)
	s.worker.Notify(v.ID)
	return v, nil
}

func (s *UploadService) mustOwnSession(ctx context.Context, userID, sessionID uint) (*model.UploadSession, error) {
	sess, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrSessionGone
		}
		return nil, err
	}
	if sess.UserID != userID {
		return nil, ErrForbiddenOwner
	}
	return sess, nil
}

type VideoService struct {
	videos *repository.VideoRepo
}

func NewVideoService(videos *repository.VideoRepo) *VideoService {
	return &VideoService{videos: videos}
}

func (s *VideoService) List(ctx context.Context, userID uint, manageAll bool) ([]model.Video, error) {
	if manageAll {
		return s.videos.List(ctx, nil)
	}
	return s.videos.List(ctx, &userID)
}

func (s *VideoService) Get(ctx context.Context, id uint) (*model.Video, error) {
	v, err := s.videos.FindByID(ctx, id)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrVideoGone
		}
		return nil, err
	}
	return v, nil
}

func (s *VideoService) FileForPlay(ctx context.Context, id uint, quality string) (*model.Video, string, error) {
	v, path, err := s.fileFor(ctx, id, quality)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, "", ErrNoRendition
	}
	if err := s.videos.IncrementView(ctx, id); err != nil {
		return nil, "", err
	}
	return v, path, nil
}

func (s *VideoService) FileForDownload(ctx context.Context, id uint, quality string) (*model.Video, string, error) {
	v, path, err := s.fileFor(ctx, id, quality)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, "", ErrNoRendition
	}
	if err := s.videos.IncrementDownload(ctx, id); err != nil {
		return nil, "", err
	}
	return v, path, nil
}

func (s *VideoService) fileFor(ctx context.Context, id uint, quality string) (*model.Video, string, error) {
	v, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if v.Status != model.VideoStatusReady && quality == "" {
		if v.SourcePath != "" {
			return v, v.SourcePath, nil
		}
		return nil, "", ErrNotReady
	}
	path := pickRendition(v, quality)
	if path == "" {
		if quality == "" && v.SourcePath != "" {
			return v, v.SourcePath, nil
		}
		return nil, "", ErrNoRendition
	}
	return v, path, nil
}

func pickRendition(v *model.Video, quality string) string {
	order := []string{"1080", "720", "480"}
	if quality != "" {
		for _, r := range v.Renditions {
			if r.Quality == quality && r.Status == model.RenditionReady && r.Path != "" {
				return r.Path
			}
		}
		return ""
	}
	for _, q := range order {
		for _, r := range v.Renditions {
			if r.Quality == q && r.Status == model.RenditionReady && r.Path != "" {
				return r.Path
			}
		}
	}
	return v.SourcePath
}
