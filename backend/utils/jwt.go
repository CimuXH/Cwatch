package utils

import (
	"github.com/golang-jwt/jwt/v5"
	"time"
)

// JWT密钥，用于签名和验证令牌
// 生产环境应该使用环境变量或配置文件管理
var secretKey = []byte("cwatch-secret-key-2026")

// MyClaims 自定义JWT声明
type MyClaims struct {
	Username string `json:"username"` // 用户名
	jwt.RegisteredClaims            // 标准声明（过期时间、签发者等）
}

// GenerateJWT 生成JWT令牌
// 参数：用户名
// 返回：JWT令牌字符串和错误
func GenerateJWT(username string) (string, error) {
	// 创建声明
	claims := MyClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),      // 令牌24h后过期
			IssuedAt:  jwt.NewNumericDate(time.Now()),                          // 签发时间
			Issuer:    "cwatch",                                                 // 签发者
		},
	}

	// 使用HS256算法创建令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 使用密钥签名生成最终的令牌字符串
	return token.SignedString(secretKey)
}

// ValidateJWT 验证JWT令牌
// 参数：JWT令牌字符串
// 返回：解析后的声明和错误
func ValidateJWT(tokenString string) (*MyClaims, error) {
	// 解析令牌
	token, err := jwt.ParseWithClaims(tokenString, &MyClaims{},
		func(token *jwt.Token) (interface{}, error) {
			// 返回用于验证的密钥
			return secretKey, nil
		})

	// 检查解析是否出错
	if err != nil {
		return nil, err
	}

	// 检查令牌是否有效
	if !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}

	// 提取声明
	claims, ok := token.Claims.(*MyClaims)
	if !ok {
		return nil, jwt.ErrTokenInvalidClaims
	}

	// 返回解析成功的声明数据
	return claims, nil
}
