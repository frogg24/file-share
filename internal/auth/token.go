package auth

import (
	"crypto/ed25519"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenIssuer struct {
	privateKey ed25519.PrivateKey
	ttl        time.Duration
}

func NewTokenIssuer(keyPath string, ttl time.Duration) (*TokenIssuer, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	if len(keyBytes) != ed25519.PrivateKeySize {
		return nil, errors.New("некорректный размер закрытого ключа")
	}

	if ttl <= 0 {
		return nil, errors.New("срок действия токена должен быть положительным")
	}

	return &TokenIssuer{
		privateKey: ed25519.PrivateKey(keyBytes),
		ttl:        ttl,
	}, nil
}

func (t *TokenIssuer) Issue(userID string) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    "file-share-auth",
		Audience:  jwt.ClaimStrings{"file-share"},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(t.privateKey)
}
