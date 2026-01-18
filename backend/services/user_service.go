package services

import (
	"backend/models"
	"backend/utils"
	"errors"
	"strings"
)

// UserService 用户服务层
// 负责处理用户相关的业务逻辑
type UserService struct{}

// RegisterRequest 注册请求结构
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=20"` // 用户名，必填，3-20字符
	Password string `json:"password" binding:"required,min=6"`        // 密码，必填，至少6字符
}

// LoginRequest 登录请求结构
type LoginRequest struct {
	Username string `json:"username" binding:"required"` // 用户名，必填
	Password string `json:"password" binding:"required"` // 密码，必填
}

// UserResponse 用户响应结构（不包含密码）
type UserResponse struct {
	ID        uint   `json:"id"`              // 用户ID
	Username  string `json:"username"`        // 用户名
	AvatarURL string `json:"avatar_url"`      // 头像URL
	Token     string `json:"token,omitempty"` // JWT令牌（仅登录时返回）
}

// Register 注册新用户
// 参数：注册请求
// 返回：用户信息和错误
func (s *UserService) Register(req RegisterRequest) (*UserResponse, error) {
	// 验证用户名
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 20 {
		return nil, errors.New("用户名长度必须在3-20个字符之间")
	}

	// 验证密码
	if len(req.Password) < 6 {
		return nil, errors.New("密码长度至少6个字符")
	}

	// 检查用户名是否已存在
	if utils.UserExists(username) {
		return nil, errors.New("用户名已存在")
	}

	// 加密密码
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	// 创建用户对象
	user := &models.User{
		Username: username,
		Password: hashedPassword,
	}

	// 保存到数据库
	if err := utils.CreateUser(user); err != nil {
		return nil, errors.New("用户创建失败")
	}

	// 返回用户信息（不包含密码）
	return &UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		AvatarURL: user.AvatarURL,
	}, nil
}

// Login 用户登录
// 参数：登录请求
// 返回：用户信息（包含JWT令牌）和错误
func (s *UserService) Login(req LoginRequest) (*UserResponse, error) {
	// 验证输入
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}

	// 查找用户
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 验证密码
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		return nil, errors.New("密码错误")
	}

	// 生成JWT令牌
	token, err := utils.GenerateJWT(user.Username)
	if err != nil {
		return nil, errors.New("令牌生成失败")
	}

	// 返回用户信息和令牌
	return &UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		AvatarURL: user.AvatarURL,
		Token:     token,
	}, nil
}

// GetUserByUsername 根据用户名获取用户信息
// 参数：用户名
// 返回：用户信息和错误
func (s *UserService) GetUserByUsername(username string) (*UserResponse, error) {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	return &UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		AvatarURL: user.AvatarURL,
	}, nil
}
