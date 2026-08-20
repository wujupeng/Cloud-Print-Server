package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string, cost int) (hash string, salt string, err error) {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return "", "", fmt.Errorf("bcrypt cost 超出范围: %d", cost)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(h), "", nil
}

func VerifyPassword(password, hash, salt string) bool {
	if hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}