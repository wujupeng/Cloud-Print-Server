package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTManagerIssueAndParse(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", 12)

	token, expiresAt, err := mgr.IssueToken("user-1", "ADMIN")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now()))

	claims, err := mgr.ParseToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.UserID)
	assert.Equal(t, "ADMIN", claims.Role)
	assert.Equal(t, "user-1", claims.Subject)
}

func TestJWTManagerExpiredToken(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", 0)
	assert.Equal(t, 12*time.Hour, mgr.TTL())

	mgr2 := &JWTManager{secret: []byte("test-secret-key"), ttl: -1 * time.Hour}
	token, _, err := mgr2.IssueToken("user-1", "USER")
	require.NoError(t, err)

	_, err = mgr2.ParseToken(token)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestJWTManagerInvalidSignature(t *testing.T) {
	mgr1 := NewJWTManager("secret-A", 12)
	mgr2 := NewJWTManager("secret-B", 12)

	token, _, err := mgr1.IssueToken("user-1", "USER")
	require.NoError(t, err)

	_, err = mgr2.ParseToken(token)
	assert.Error(t, err)
}

func TestJWTManagerInvalidToken(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", 12)

	_, err := mgr.ParseToken("not-a-jwt")
	assert.Error(t, err)

	_, err = mgr.ParseToken("")
	assert.Error(t, err)

	_, err = mgr.ParseToken("a.b.c")
	assert.Error(t, err)
}

func TestJWTManagerDifferentUsers(t *testing.T) {
	mgr := NewJWTManager("test-secret", 12)

	t1, _, err := mgr.IssueToken("user-1", "USER")
	require.NoError(t, err)
	t2, _, err := mgr.IssueToken("user-2", "ADMIN")
	require.NoError(t, err)

	c1, err := mgr.ParseToken(t1)
	require.NoError(t, err)
	c2, err := mgr.ParseToken(t2)
	require.NoError(t, err)

	assert.NotEqual(t, c1.UserID, c2.UserID)
	assert.NotEqual(t, c1.Role, c2.Role)
}

func TestHashAndVerifyPassword(t *testing.T) {
	password := "MySecure@Password123"
	hash, salt, err := HashPassword(password, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Empty(t, salt)

	assert.True(t, VerifyPassword(password, hash, salt))
	assert.False(t, VerifyPassword("wrong-password", hash, salt))
	assert.False(t, VerifyPassword("", hash, salt))
	assert.False(t, VerifyPassword(password, "", salt))
}

func TestHashPasswordDefaultCost(t *testing.T) {
	hash, _, err := HashPassword("test", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.True(t, VerifyPassword("test", hash, ""))
}

func TestHashPasswordInvalidCost(t *testing.T) {
	_, _, err := HashPassword("test", 1)
	assert.Error(t, err)

	_, _, err = HashPassword("test", 100)
	assert.Error(t, err)
}

func TestHashPasswordDifferentHashes(t *testing.T) {
	h1, _, err := HashPassword("same-password", 10)
	require.NoError(t, err)
	h2, _, err := HashPassword("same-password", 10)
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2)
	assert.True(t, VerifyPassword("same-password", h1, ""))
	assert.True(t, VerifyPassword("same-password", h2, ""))
}

func TestJWTMiddlewareMissingHeader(t *testing.T) {
	mgr := NewJWTManager("secret", 12)
	mw := JWTMiddleware(mgr)
	assert.NotNil(t, mw)
}

func TestAdminOnlyMiddleware(t *testing.T) {
	mw := AdminOnlyMiddleware(nil)
	assert.NotNil(t, mw)
}