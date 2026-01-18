package utils

import (
	"golang.org/x/crypto/bcrypt"
	"log"
)

// HashPassword 使用bcrypt加密密码
// 参数：明文密码
// 返回：加密后的密码哈希值和错误
func HashPassword(password string) (string, error) {
	// 使用bcrypt默认成本因子生成密码哈希
	// bcrypt.DefaultCost = 10，成本越高越安全但也越慢
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Println("密码hash错误：", err)
		return "", err
	}

	return string(bytes), nil
}

// CheckPasswordHash 校验密码是否匹配
// 参数：明文密码，密码哈希值
// 返回：是否匹配
func CheckPasswordHash(password string, hash string) bool {
	// 比较明文密码和哈希值
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil // 如果没有错误，说明密码匹配
}
