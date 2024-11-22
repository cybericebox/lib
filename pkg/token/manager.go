package token

import (
	"encoding/base64"
	"github.com/cybericebox/lib/pkg/appError"
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt"
	"time"
)

const (
	tokenTypeAccess int8 = iota
	tokenTypeRefresh
)

type Manager struct {
	signingKey string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(signingKey string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if signingKey == "" {
		return nil, appError.ErrTokenEmptySignature.Err()
	}

	if accessTTL <= 0 || refreshTTL <= 0 {
		return nil, appError.ErrTokenTTLMustBeGreaterThanZero.Err()
	}

	return &Manager{signingKey: signingKey, accessTTL: accessTTL, refreshTTL: refreshTTL}, nil
}

func (m *Manager) NewAccessToken(subject interface{}, ttl ...time.Duration) (string, error) {
	// If no TTL is provided, use the default access TTL
	accessTTL := m.accessTTL

	// If a TTL is provided, use that instead
	if len(ttl) == 1 {
		accessTTL = ttl[0]
	}
	token, err := m.newToken(subject, accessTTL, tokenTypeAccess)
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to create access token").Err()
	}

	return token, nil
}

func (m *Manager) NewRefreshToken(subject interface{}, ttl ...time.Duration) (string, error) {
	// If no TTL is provided, use the default refresh TTL
	refreshTTL := m.refreshTTL

	// If a TTL is provided, use that instead
	if len(ttl) == 1 {
		refreshTTL = ttl[0]
	}

	token, err := m.newToken(subject, refreshTTL, tokenTypeRefresh)
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to create refresh token").Err()
	}

	return token, nil
}

func (m *Manager) NewBase64Token(subject interface{}, ttl time.Duration) (string, error) {
	strToken, err := m.newToken(subject, ttl)
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to create token").Err()
	}

	bs64Token := base64.StdEncoding.EncodeToString([]byte(strToken))

	return bs64Token, nil
}

func (m *Manager) ParseAccessToken(Token string) (interface{}, error) {
	// Parse the token
	token, err := m.parseToken(Token)
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to parse token").Err()
	}

	// Check if the token is valid
	claims, ok := token.Claims.(jwt.MapClaims)
	// If the token is not valid, return an error
	if !ok || int8(claims["token"].(float64)) != tokenTypeAccess {
		return "", appError.ErrTokenInvalidJWTToken.Err()
	}

	return claims["sub"], nil
}

func (m *Manager) ParseRefreshToken(Token string) (interface{}, error) {
	// Parse the token
	token, err := m.parseToken(Token)
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to parse token").Err()
	}

	// Check if the token is valid
	claims, ok := token.Claims.(jwt.MapClaims)
	// If the token is not valid, return an error
	if !ok || int8(claims["token"].(float64)) != tokenTypeRefresh {
		return "", appError.ErrTokenInvalidJWTToken.Err()
	}

	return claims["sub"], nil
}

func (m *Manager) ParseBase64Token(base64Token string) (interface{}, error) {
	// Decode the base64 token
	Token, err := base64.StdEncoding.DecodeString(base64Token)
	if err != nil {
		return nil, appError.ErrToken.WithError(err).WithMessage("Failed to decode base64 token").Err()
	}

	// Parse the token
	token, err := m.parseToken(string(Token))
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to parse token").Err()
	}

	// Check if the token is valid
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", appError.ErrTokenInvalidJWTToken.Err()
	}

	return claims["sub"], nil
}

func (m *Manager) GetAccessTokenTTL() time.Duration {
	return m.accessTTL
}

func (m *Manager) GetRefreshTokenTTL() time.Duration {
	return m.refreshTTL
}

func (m *Manager) newToken(subject interface{}, tokenTTL time.Duration, tokenType ...int8) (string, error) {
	tokenClaims := jwt.MapClaims{}
	tokenClaims["exp"] = time.Now().Add(tokenTTL).Unix()
	tokenClaims["jti"] = uuid.Must(uuid.NewV4())
	tokenClaims["sub"] = subject
	// If a token type is provided, add it to the token
	if len(tokenType) > 0 {
		tokenClaims["token"] = tokenType[0]
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)

	signed, err := token.SignedString([]byte(m.signingKey))
	if err != nil {
		return "", appError.ErrToken.WithError(err).WithMessage("Failed to sign token").Err()
	}

	return signed, nil
}

func (m *Manager) parseToken(Token string) (*jwt.Token, error) {
	return jwt.Parse(Token, func(token *jwt.Token) (i interface{}, err error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, appError.ErrToken.WithMessage("Unexpected signing method").WithContext("method", token.Header["alg"]).Err()
		}

		return []byte(m.signingKey), nil
	})
}
