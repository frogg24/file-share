package token

import (
	"crypto/ed25519"
	"errors"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("недействительный токен")

type Verifier struct {
	publicKey ed25519.PublicKey
}

func NewVerifier(keyPath string) (*Verifier, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, errors.New("некорректный размер закрытого ключа")
	}

	return &Verifier{
		publicKey: ed25519.PublicKey(keyBytes),
	}, nil
}

func (v *Verifier) Verify(rawToken string) (string, error) {
	var claims jwt.RegisteredClaims

	parsedToken, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		func(token *jwt.Token) (any, error) {
			return v.publicKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer("file-share-auth"),
		jwt.WithAudience("file-share"),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return "", ErrInvalidToken
	}

	if !parsedToken.Valid {
		return "", ErrInvalidToken
	}

	if claims.Subject == "" || claims.IssuedAt == nil {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}
