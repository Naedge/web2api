package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"web2api/internal/model"
	"web2api/internal/repository"
)

const SessionCookieName = "web2api_session"

type AuthService struct {
	repo          *repository.AdminUserRepository
	sessionSecret []byte
}

type SessionInfo struct {
	UserID    uint
	Username  string
	ExpiresAt time.Time
}

type IssuedSession struct {
	Value     string
	ExpiresAt time.Time
	Username  string
}

func NewAuthService(
	repo *repository.AdminUserRepository,
	sessionSecret string,
) *AuthService {
	return &AuthService{
		repo:          repo,
		sessionSecret: []byte(strings.TrimSpace(sessionSecret)),
	}
}

func (s *AuthService) IsInitialized(ctx context.Context) (bool, error) {
	count, err := s.repo.Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *AuthService) Setup(
	ctx context.Context,
	username string,
	password string,
) (*IssuedSession, error) {
	initialized, err := s.IsInitialized(ctx)
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, &StatusError{Code: 409, Message: "admin user already initialized"}
	}

	username, password, err = validateCredential(username, password)
	if err != nil {
		return nil, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.AdminUser{
		Username:     username,
		PasswordHash: string(passwordHash),
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.issueSession(user)
}

func (s *AuthService) Login(
	ctx context.Context,
	username string,
	password string,
) (*IssuedSession, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil, badRequest("username and password are required")
	}

	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, unauthorized("用户名或密码错误")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, unauthorized("用户名或密码错误")
	}

	return s.issueSession(user)
}

func (s *AuthService) ParseSession(
	ctx context.Context,
	cookieValue string,
) (*SessionInfo, error) {
	cookieValue = strings.TrimSpace(cookieValue)
	if cookieValue == "" {
		return nil, unauthorized("未登录")
	}

	raw, err := base64.RawURLEncoding.DecodeString(cookieValue)
	if err != nil {
		return nil, unauthorized("登录态无效")
	}

	payload, signature, ok := strings.Cut(string(raw), ".")
	if !ok {
		return nil, unauthorized("登录态无效")
	}

	expected := s.sign(payload)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return nil, unauthorized("登录态无效")
	}

	parts := strings.Split(payload, "|")
	if len(parts) != 4 {
		return nil, unauthorized("登录态无效")
	}

	userID64, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return nil, unauthorized("登录态无效")
	}
	expiresAtUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, unauthorized("登录态无效")
	}

	expiresAt := time.Unix(expiresAtUnix, 0)
	if time.Now().After(expiresAt) {
		return nil, unauthorized("登录已过期")
	}

	user, err := s.repo.GetByID(ctx, uint(userID64))
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, unauthorized("用户不存在")
	}

	return &SessionInfo{
		UserID:    uint(userID64),
		Username:  user.Username,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *AuthService) issueSession(user *model.AdminUser) (*IssuedSession, error) {
	if len(s.sessionSecret) == 0 {
		return nil, fmt.Errorf("session secret is empty")
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	nonce := randomHex(12)
	payload := fmt.Sprintf("%d|%s|%d|%s", user.ID, user.Username, expiresAt.Unix(), nonce)
	signature := s.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "." + signature))

	return &IssuedSession{
		Value:     value,
		ExpiresAt: expiresAt,
		Username:  user.Username,
	}, nil
}

func (s *AuthService) sign(payload string) string {
	mac := hmac.New(sha256.New, s.sessionSecret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func validateCredential(username string, password string) (string, string, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)

	if username == "" || password == "" {
		return "", "", badRequest("username and password are required")
	}
	if len(username) < 3 {
		return "", "", badRequest("username must be at least 3 characters")
	}
	if len(password) < 3 {
		return "", "", badRequest("password must be at least 3 characters")
	}

	return username, password, nil
}

func randomHex(length int) string {
	size := length/2 + 1
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return newHexID(length)
	}
	return hex.EncodeToString(buf)[:length]
}
