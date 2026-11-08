package token_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frogg24/file-share/internal/auth"
	"github.com/frogg24/file-share/internal/platform/token"
	"github.com/golang-jwt/jwt/v5"
)

func TestVerifierAcceptsIssuedToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.key")
	publicPath := filepath.Join(dir, "public.key")

	err = os.WriteFile(privatePath, privateKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(publicPath, publicKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	issuer, err := auth.NewTokenIssuer(privatePath, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}

	const expectedUserID = "test-user-id"

	rawToken, err := issuer.Issue(expectedUserID)
	if err != nil {
		t.Fatal(err)
	}

	actualUserID, err := verifier.Verify(rawToken)
	if err != nil {
		t.Fatalf("valid token was rejected: %v", err)
	}

	if actualUserID != expectedUserID {
		t.Fatalf(
			"expected user ID %q, got %q",
			expectedUserID,
			actualUserID,
		)
	}
}

func TestVerifierRejectsWrongKey(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.key")
	publicPath := filepath.Join(dir, "public.key")

	err = os.WriteFile(privatePath, privateKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(publicPath, publicKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	issuer, err := auth.NewTokenIssuer(privatePath, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}

	const expectedUserID = "test-user-id"

	rawToken, err := issuer.Issue(expectedUserID)
	if err != nil {
		t.Fatal(err)
	}

	actualUserID, err := verifier.Verify(rawToken)

	if !errors.Is(err, token.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}

	if actualUserID != "" {
		t.Fatalf("rejected token returned user ID %q", actualUserID)
	}

}

func TestVerifierRejectsExpiredToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	publicPath := filepath.Join(t.TempDir(), "public.key")

	err = os.WriteFile(publicPath, publicKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	claims := jwt.RegisteredClaims{
		Subject:   "test-user-id",
		Issuer:    "file-share-auth",
		Audience:  jwt.ClaimStrings{"file-share"},
		IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
	}

	signedToken := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	rawToken, err := signedToken.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	userID, err := verifier.Verify(rawToken)

	if !errors.Is(err, token.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}

	if userID != "" {
		t.Fatalf("expired token returned user ID %q", userID)
	}
}

func TestVerifierRejectsInvalidClaims(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	publicPath := filepath.Join(t.TempDir(), "public.key")

	err = os.WriteFile(publicPath, publicKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []string{
		"missing_expiration",
		"wrong_issuer",
		"missing_issuer",
		"wrong_audience",
		"missing_audience",
		"missing_subject",
		"missing_issued_at",
		"future_issued_at",
		"wrong_algorithm",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			now := time.Now()

			claims := jwt.RegisteredClaims{
				Subject:   "test-user-id",
				Issuer:    "file-share-auth",
				Audience:  jwt.ClaimStrings{"file-share"},
				IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			}

			switch name {
			case "missing_expiration":
				claims.ExpiresAt = nil
			case "wrong_issuer":
				claims.Issuer = "another-service"
			case "missing_issuer":
				claims.Issuer = ""
			case "wrong_audience":
				claims.Audience = jwt.ClaimStrings{"another-system"}
			case "missing_audience":
				claims.Audience = nil
			case "missing_subject":
				claims.Subject = ""
			case "missing_issued_at":
				claims.IssuedAt = nil
			case "future_issued_at":
				claims.IssuedAt = jwt.NewNumericDate(
					now.Add(10 * time.Minute),
				)
			}

			signedToken := jwt.NewWithClaims(
				jwt.SigningMethodEdDSA,
				claims,
			)
			var signingKey any = privateKey

			if name == "wrong_algorithm" {
				signedToken = jwt.NewWithClaims(
					jwt.SigningMethodHS256,
					claims,
				)
				signingKey = []byte("test-only-secret")
			}

			rawToken, err := signedToken.SignedString(signingKey)
			if err != nil {
				t.Fatal(err)
			}

			userID, err := verifier.Verify(rawToken)

			if !errors.Is(err, token.ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}

			if userID != "" {
				t.Fatalf("rejected token returned user ID %q", userID)
			}
		})
	}
}
