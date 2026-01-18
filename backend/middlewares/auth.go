package middlewares

import (
	"backend/utils"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

// AuthMiddleware JWT认证中间件
// 用于保护需要登录才能访问的API
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头获取Authorization字段
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "缺少认证令牌",
			})
			c.Abort() // 终止请求处理
			return
		}

		// 检查Bearer前缀
		// 标准格式：Authorization: Bearer <token>
		parts := strings.SplitN(authHeader, " ", 2)  // 把字符串分割成两部分
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌格式错误",
			})
			c.Abort()
			return
		}

		// 提取令牌
		token := parts[1]

		// 检查 token 是否在黑名单中
		if utils.IsBlacklisted(token) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "令牌已失效，请重新登录",
			})
			c.Abort()
			return
		}

		// 验证JWT令牌
		claims, err := utils.ValidateJWT(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的认证令牌",
			})
			c.Abort()
			return
		}

		// 将用户名和 token 存入上下文，供后续处理函数使用
		c.Set("username", claims.Username)
		c.Set("token", token)

		// 继续处理请求
		c.Next()
	}
}
