package handler

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"videoview/internal/middleware"
	"videoview/internal/model"
	"videoview/internal/pkg/response"
	"videoview/internal/service"
)

type UploadHandler struct {
	uploads *service.UploadService
}

func NewUploadHandler(uploads *service.UploadService) *UploadHandler {
	return &UploadHandler{uploads: uploads}
}

func (h *UploadHandler) Init(c *gin.Context) {
	var req struct {
		Filename  string `json:"filename" binding:"required"`
		Size      int64  `json:"size" binding:"required"`
		ChunkSize int64  `json:"chunk_size"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	sess, err := h.uploads.Init(c.Request.Context(), middleware.UserID(c), req.Filename, req.Size, req.ChunkSize)
	if err != nil {
		writeUploadErr(c, err)
		return
	}
	response.OK(c, sessionView(sess, nil))
}

func (h *UploadHandler) PutChunk(c *gin.Context) {
	sid, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	idx, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		response.BadRequest(c, "invalid chunk index")
		return
	}
	sess, err := h.uploads.PutChunk(c.Request.Context(), middleware.UserID(c), sid, idx, c.Request.Body)
	if err != nil {
		writeUploadErr(c, err)
		return
	}
	response.OK(c, sessionView(sess, nil))
}

func (h *UploadHandler) Get(c *gin.Context) {
	sid, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	sess, chunks, err := h.uploads.Get(c.Request.Context(), middleware.UserID(c), sid)
	if err != nil {
		writeUploadErr(c, err)
		return
	}
	response.OK(c, sessionView(sess, chunks))
}

func (h *UploadHandler) Complete(c *gin.Context) {
	sid, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	v, err := h.uploads.Complete(c.Request.Context(), middleware.UserID(c), sid)
	if err != nil {
		writeUploadErr(c, err)
		return
	}
	response.OK(c, v)
}

func writeUploadErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBadExt), errors.Is(err, service.ErrBadChunk), errors.Is(err, service.ErrIncomplete):
		response.BadRequest(c, err.Error())
	case errors.Is(err, service.ErrSessionGone), errors.Is(err, service.ErrVideoGone):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrForbiddenOwner):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrSessionDone):
		response.BadRequest(c, err.Error())
	default:
		response.ServerError(c, err.Error())
	}
}

type VideoHandler struct {
	videos *service.VideoService
}

func NewVideoHandler(videos *service.VideoService) *VideoHandler {
	return &VideoHandler{videos: videos}
}

func (h *VideoHandler) List(c *gin.Context) {
	manage := middleware.HasPerm(c, model.PermVideoManage)
	list, err := h.videos.List(c.Request.Context(), middleware.UserID(c), manage)
	if err != nil {
		response.ServerError(c, "list videos failed")
		return
	}
	response.OK(c, list)
}

func (h *VideoHandler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	v, err := h.videos.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrVideoGone) {
			response.NotFound(c, err.Error())
			return
		}
		response.ServerError(c, "get video failed")
		return
	}
	response.OK(c, v)
}

func (h *VideoHandler) Play(c *gin.Context) {
	h.serveFile(c, false)
}

func (h *VideoHandler) Download(c *gin.Context) {
	h.serveFile(c, true)
}

func (h *VideoHandler) serveFile(c *gin.Context, download bool) {
	id, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	quality := c.Query("quality")
	var (
		v    *model.Video
		path string
	)
	if download {
		v, path, err = h.videos.FileForDownload(c.Request.Context(), id, quality)
	} else {
		v, path, err = h.videos.FileForPlay(c.Request.Context(), id, quality)
	}
	if err != nil {
		switch {
		case errors.Is(err, service.ErrVideoGone):
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrNotReady), errors.Is(err, service.ErrNoRendition):
			response.BadRequest(c, err.Error())
		default:
			response.ServerError(c, err.Error())
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		response.NotFound(c, "file missing")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		response.ServerError(c, "stat file failed")
		return
	}
	if download {
		name := v.OriginalFilename
		if quality != "" {
			ext := filepath.Ext(name)
			base := name[:len(name)-len(ext)]
			if ext == "" {
				base = name
			}
			name = base + "_" + quality + ".mp4"
		}
		c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), stat.ModTime(), f)
}
