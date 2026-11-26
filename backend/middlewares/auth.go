package middlewares

import (
	"errors"
	"net/http"
	"strings"

	"backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AuthMiddleware() gin.HandlerFunc {
	return authenticate(true)
}

func OptionalAuthMiddleware() gin.HandlerFunc {
	return authenticate(false)
}

func authenticate(required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, _ := strings.Cut(c.GetHeader("Authorization"), " ")
		if strings.EqualFold(scheme, "Bearer") && len(token) == 64 {
			username, err := utils.SessionUsername(c.Request.Context(), token)
			if err == nil {
				c.Set("username", username)
				c.Set("token", token)
				c.Next()
				return
			}
			if required && !errors.Is(err, gorm.ErrRecordNotFound) {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "认证服务暂时不可用"})
				return
			}
		}
		if required {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌，请重新登录"})
			return
		}
		c.Next()
	}
}
