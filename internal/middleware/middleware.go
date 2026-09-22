package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"videoview/internal/model"
	"videoview/internal/pkg/response"
	"videoview/internal/repository"
)

type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"un"`
	jwt.RegisteredClaims
}

const (
	CtxUserID      = "user_id"
	CtxUsername    = "username"
	CtxPermissions = "permissions"
)

func JWT(secret string, users *repository.UserRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			response.Unauthorized(c, "missing token")
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			response.Unauthorized(c, "invalid token")
			c.Abort()
			return
		}
		user, err := users.FindByID(c.Request.Context(), claims.UserID)
		if err != nil || user.Status != model.UserStatusActive {
			response.Unauthorized(c, "user disabled")
			c.Abort()
			return
		}
		perms := map[string]struct{}{}
		for _, role := range user.Roles {
			for _, p := range role.Permissions {
				perms[p.Code] = struct{}{}
			}
		}
		c.Set(CtxUserID, user.ID)
		c.Set(CtxUsername, user.Username)
		c.Set(CtxPermissions, perms)
		c.Next()
	}
}

func Require(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := c.Get(CtxPermissions)
		if !ok {
			response.Forbidden(c, "no permission")
			c.Abort()
			return
		}
		perms := raw.(map[string]struct{})
		if _, ok := perms[code]; !ok {
			response.Forbidden(c, "no permission")
			c.Abort()
			return
		}
		c.Next()
	}
}

func UserID(c *gin.Context) uint {
	v, _ := c.Get(CtxUserID)
	id, _ := v.(uint)
	return id
}

func HasPerm(c *gin.Context, code string) bool {
	raw, ok := c.Get(CtxPermissions)
	if !ok {
		return false
	}
	perms := raw.(map[string]struct{})
	_, ok = perms[code]
	return ok
}

func RequestLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
}

func Recovery(log *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic", zap.Any("recover", recovered))
		c.AbortWithStatusJSON(http.StatusInternalServerError, response.Body{
			Code:    50000,
			Message: "internal error",
		})
	})
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
