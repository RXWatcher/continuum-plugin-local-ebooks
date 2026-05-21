// Package tokens verifies signed media URL tokens (?token=<HS256 JWT>) minted
// by the ebooks portal. Browsers can't send Authorization headers on
// <img>/<a download>/<iframe> tag requests, so the portal embeds a short-TTL
// signed JWT in the URL and the byte routes validate it here.
package tokens

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Audience is the canonical `aud` claim shared by every ebook backend.
const Audience = "ebook_backend"

// CoverFileIdx is the sentinel file_idx claim value used for cover tokens.
const CoverFileIdx = -1

// FileFileIdx is the claim value for the book's primary file. Ebooks are
// single-file per book (one format stored per book row).
const FileFileIdx = 0

// Claims is the verified subset of claims callers act on.
type Claims struct {
	UserID  string
	BookID  string
	FileIdx int
}

// ErrTokenMissing is returned when no token was supplied.
var ErrTokenMissing = errors.New("media token missing")

// ErrSecretUnconfigured is returned when verification is attempted with an
// empty signing secret.
var ErrSecretUnconfigured = errors.New("media signing secret not configured")

// Verify parses + verifies tokenStr against secret. expectedBookID and
// expectedFileIdx must match the token's claims.
func Verify(secret, tokenStr, expectedBookID string, expectedFileIdx int) (*Claims, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrSecretUnconfigured
	}
	if tokenStr == "" {
		return nil, ErrTokenMissing
	}
	key := decodeSecret(secret)
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	}, jwt.WithAudience(Audience), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("token invalid")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("claims not MapClaims")
	}
	bookID, _ := claims["book_id"].(string)
	if bookID == "" || bookID != expectedBookID {
		return nil, fmt.Errorf("book_id mismatch")
	}
	fidx, ok := claims["file_idx"].(float64)
	if !ok || int(fidx) != expectedFileIdx {
		return nil, fmt.Errorf("file_idx mismatch")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, errors.New("sub required")
	}
	return &Claims{UserID: sub, BookID: bookID, FileIdx: expectedFileIdx}, nil
}

func decodeSecret(secret string) []byte {
	if b, err := base64.StdEncoding.DecodeString(secret); err == nil && len(b) > 0 {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(secret); err == nil && len(b) > 0 {
		return b
	}
	return []byte(secret)
}
