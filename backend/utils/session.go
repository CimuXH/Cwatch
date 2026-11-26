package utils

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"backend/models"
)

func sessionHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func CreateSession(userID uint) (string, error) {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(random[:])
	session := models.Session{TokenHash: sessionHash(token), UserID: userID, ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
	return token, db.Create(&session).Error
}

func SessionUsername(ctx context.Context, token string) (string, error) {
	var user models.User
	err := db.WithContext(ctx).Select("users.username").
		Joins("JOIN sessions ON sessions.user_id = users.id").
		Where("sessions.token_hash = ? AND sessions.expires_at > ?", sessionHash(token), time.Now().UTC()).
		Take(&user).Error
	return user.Username, err
}

func RevokeSession(ctx context.Context, token string) error {
	return db.WithContext(ctx).Where("token_hash = ?", sessionHash(token)).Delete(&models.Session{}).Error
}
