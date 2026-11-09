package gateway

import (
	"context"

	"github.com/frogg24/file-share/internal/platform/httpauth"
	"github.com/frogg24/file-share/internal/platform/token"
)

type Authenticator = httpauth.Authenticator

func NewAuthenticator(verifier *token.Verifier) *Authenticator {
	return httpauth.NewAuthenticator(verifier)
}

func UserID(ctx context.Context) (string, bool) {
	return httpauth.UserID(ctx)
}
