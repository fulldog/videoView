package handler

import (
	"github.com/gin-gonic/gin"

	"videoview/internal/middleware"
	"videoview/internal/model"
	"videoview/internal/repository"
)

type Deps struct {
	Auth    *AuthHandler
	RBAC    *RBACHandler
	Uploads *UploadHandler
	Videos  *VideoHandler
	Users   *repository.UserRepo
	Secret  string
	IsDev   bool
}

func RegisterRoutes(r *gin.Engine, d Deps, requestLog gin.HandlerFunc, recovery gin.HandlerFunc) {
	r.Use(recovery)
	r.Use(requestLog)
	r.Use(middleware.CORS())

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		auth.POST("/register", d.Auth.Register)
		auth.POST("/login", d.Auth.Login)
		auth.POST("/password/reset-request", d.Auth.ResetRequest)
		auth.POST("/password/reset", d.Auth.Reset)

		needAuth := v1.Group("")
		needAuth.Use(middleware.JWT(d.Secret, d.Users))
		needAuth.POST("/auth/password/change", d.Auth.ChangePassword)

		rbac := needAuth.Group("/rbac")
		rbac.Use(middleware.Require(model.PermRBACManage))
		rbac.GET("/users", d.RBAC.Users)
		rbac.GET("/roles", d.RBAC.Roles)
		rbac.POST("/roles", d.RBAC.CreateRole)
		rbac.PUT("/roles/:id", d.RBAC.UpdateRole)
		rbac.GET("/permissions", d.RBAC.Perms)
		rbac.PUT("/users/:id/roles", d.RBAC.SetUserRoles)

		up := needAuth.Group("/uploads")
		up.Use(middleware.Require(model.PermVideoUpload))
		up.POST("", d.Uploads.Init)
		up.PUT("/:id/chunks/:index", d.Uploads.PutChunk)
		up.GET("/:id", d.Uploads.Get)
		up.POST("/:id/complete", d.Uploads.Complete)

		vid := needAuth.Group("/videos")
		vid.GET("", middleware.Require(model.PermVideoRead), d.Videos.List)
		vid.GET("/:id", middleware.Require(model.PermVideoRead), d.Videos.Get)
		vid.GET("/:id/play", middleware.Require(model.PermVideoRead), d.Videos.Play)
		vid.GET("/:id/download", middleware.Require(model.PermVideoDownload), d.Videos.Download)
	}
}
