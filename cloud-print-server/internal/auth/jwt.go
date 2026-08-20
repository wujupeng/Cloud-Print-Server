package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttlHours int) *JWTManager {
	ttl := time.Duration(ttlHours) * time.Hour
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &JWTManager{secret: []byte(secret), ttl: ttl}
}

func (m *JWTManager) IssueToken(userID, role string) (token string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	expiresAt = now.Add(m.ttl)
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   userID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *JWTManager) ParseToken(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, ErrTokenInvalid
		}
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

func (m *JWTManager) TTL() time.Duration { return m.ttl }