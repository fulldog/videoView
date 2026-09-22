package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"videoview/internal/middleware"
	"videoview/internal/model"
	"videoview/internal/pkg/response"
	"videoview/internal/repository"
	"videoview/internal/service"
)

type AuthHandler struct {
	auth *service.AuthService
}

func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

type userPassReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req userPassReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	user, token, err := h.auth.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeAuthErr(c, err)
		return
	}
	response.OK(c, gin.H{"token": token, "user": service.PublicUser(user)})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req userPassReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	user, token, err := h.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeAuthErr(c, err)
		return
	}
	response.OK(c, gin.H{"token": token, "user": service.PublicUser(user)})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), middleware.UserID(c), req.OldPassword, req.NewPassword); err != nil {
		writeAuthErr(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *AuthHandler) ResetRequest(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	token, returned, err := h.auth.RequestReset(c.Request.Context(), req.Username)
	if err != nil {
		writeAuthErr(c, err)
		return
	}
	data := gin.H{"message": "if the account exists, a reset token was issued"}
	if returned {
		data["token"] = token
	}
	response.OK(c, data)
}

func (h *AuthHandler) Reset(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	if err := h.auth.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		writeAuthErr(c, err)
		return
	}
	response.OK(c, nil)
}

func writeAuthErr(c *gin.Context, err error) {
	code, msg := service.MapAuthError(err)
	status := http.StatusBadRequest
	if code == 40100 {
		status = http.StatusUnauthorized
	}
	if code == 50000 {
		response.ServerError(c, msg)
		return
	}
	response.Fail(c, status, code, msg)
}

type RBACHandler struct {
	rbac *service.RBACService
}

func NewRBACHandler(rbac *service.RBACService) *RBACHandler {
	return &RBACHandler{rbac: rbac}
}

func (h *RBACHandler) Users(c *gin.Context) {
	users, err := h.rbac.ListUsers(c.Request.Context())
	if err != nil {
		response.ServerError(c, "list users failed")
		return
	}
	out := make([]any, 0, len(users))
	for i := range users {
		out = append(out, service.PublicUser(&users[i]))
	}
	response.OK(c, out)
}

func (h *RBACHandler) Roles(c *gin.Context) {
	roles, err := h.rbac.ListRoles(c.Request.Context())
	if err != nil {
		response.ServerError(c, "list roles failed")
		return
	}
	response.OK(c, roles)
}

func (h *RBACHandler) Perms(c *gin.Context) {
	perms, err := h.rbac.ListPerms(c.Request.Context())
	if err != nil {
		response.ServerError(c, "list permissions failed")
		return
	}
	response.OK(c, perms)
}

func (h *RBACHandler) CreateRole(c *gin.Context) {
	var req struct {
		Name          string `json:"name" binding:"required"`
		Code          string `json:"code" binding:"required"`
		PermissionIDs []uint `json:"permission_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	role, err := h.rbac.CreateRole(c.Request.Context(), req.Name, req.Code, req.PermissionIDs)
	if err != nil {
		response.ServerError(c, "create role failed")
		return
	}
	response.OK(c, role)
}

func (h *RBACHandler) UpdateRole(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Name          string `json:"name"`
		PermissionIDs []uint `json:"permission_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	role, err := h.rbac.UpdateRole(c.Request.Context(), id, req.Name, req.PermissionIDs)
	if err != nil {
		if repository.IsNotFound(err) {
			response.NotFound(c, "role not found")
			return
		}
		response.ServerError(c, "update role failed")
		return
	}
	response.OK(c, role)
}

func (h *RBACHandler) SetUserRoles(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		RoleIDs []uint `json:"role_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	if err := h.rbac.SetUserRoles(c.Request.Context(), id, req.RoleIDs); err != nil {
		if errors.Is(err, service.ErrRoleNotFound) || repository.IsNotFound(err) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ServerError(c, "set roles failed")
		return
	}
	response.OK(c, nil)
}

func parseID(c *gin.Context, name string) (uint, error) {
	n, err := strconv.ParseUint(c.Param(name), 10, 64)
	return uint(n), err
}

func sessionView(s *model.UploadSession, chunks []int) gin.H {
	return gin.H{
		"id":                s.ID,
		"original_filename": s.OriginalFilename,
		"stored_basename":   s.StoredBasename,
		"ext":               s.Ext,
		"total_size":        s.TotalSize,
		"chunk_size":        s.ChunkSize,
		"chunk_total":       s.ChunkTotal,
		"uploaded_chunks":   chunks,
		"upload_progress":   s.UploadProgress,
		"status":            s.Status,
		"video_id":          s.VideoID,
	}
}
