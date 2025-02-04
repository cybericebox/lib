package token

import (
	"encoding/base64"
	"github.com/cybericebox/lib/pkg/libError"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"time"
)

const (
	tokenTypeAccess int8 = iota
	tokenTypeRefresh
)

type (
	manager struct {
		signingKey string
		issuer     string
	}

	AccessRefreshTokenManager struct {
		manager    *manager
		accessTTL  time.Duration
		refreshTTL time.Duration
	}

	Base64TokenManager struct {
		manager *manager
		ttl     time.Duration
	}

	AccessRefreshTokenDependencies struct {
		SigningKey string
		Issuer     string
		AccessTTL  time.Duration
		RefreshTTL time.Duration
	}

	Base64TokenDependencies struct {
		SigningKey string
		Issuer     string
		TTL        time.Duration
	}

	dependencies struct {
		SigningKey string
		Issuer     string
	}
)

func NewAccessRefreshTokenManager(deps AccessRefreshTokenDependencies) (*AccessRefreshTokenManager, error) {
	if deps.AccessTTL < 0 || deps.RefreshTTL < 0 {
		return nil, libError.ErrTokenTTLMustBeNotNegative.Err()
	}

	m, err := newTokenManager(dependencies{SigningKey: deps.SigningKey, Issuer: deps.Issuer})
	if err != nil {
		return nil, err
	}

	return &AccessRefreshTokenManager{
		manager:    m,
		accessTTL:  deps.AccessTTL,
		refreshTTL: deps.RefreshTTL,
	}, nil
}

func (m *AccessRefreshTokenManager) NewAccessToken(subject interface{}, ttl ...time.Duration) (string, error) {
	// If no TTL is provided, use the default access TTL
	accessTTL := m.accessTTL

	// If a TTL is provided, use that instead
	if len(ttl) == 1 {
		if ttl[0] < 0 {
			return "", libError.ErrTokenTTLMustBeNotNegative.Err()
		}
		accessTTL = ttl[0]
	}
	token, err := m.manager.newToken(subject, accessTTL, tokenTypeAccess)
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to create access token").Err()
	}

	return token, nil
}

func (m *AccessRefreshTokenManager) NewRefreshToken(subject interface{}, ttl ...time.Duration) (string, error) {
	// If no TTL is provided, use the default refresh TTL
	refreshTTL := m.refreshTTL

	// If a TTL is provided, use that instead
	if len(ttl) == 1 {
		if ttl[0] < 0 {
			return "", libError.ErrTokenTTLMustBeNotNegative.Err()
		}
		refreshTTL = ttl[0]
	}

	token, err := m.manager.newToken(subject, refreshTTL, tokenTypeRefresh)
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to create refresh token").Err()
	}

	return token, nil
}

func (m *AccessRefreshTokenManager) ParseAccessToken(Token string) (interface{}, error) {
	// Parse the token
	token, err := m.manager.parseToken(Token)
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to parse token").Err()
	}

	// Check if the token is valid
	claims, ok := token.Claims.(jwt.MapClaims)
	// If the token is not valid, return an error
	if !ok || int8(claims["token"].(float64)) != tokenTypeAccess {
		return "", libError.ErrTokenInvalidJWTToken.Err()
	}

	return claims["sub"], nil
}

func (m *AccessRefreshTokenManager) ParseRefreshToken(Token string) (interface{}, error) {
	// Parse the token
	token, err := m.manager.parseToken(Token)
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to parse token").Err()
	}

	// Check if the token is valid
	claims, ok := token.Claims.(jwt.MapClaims)
	// If the token is not valid, return an error
	if !ok || int8(claims["token"].(float64)) != tokenTypeRefresh {
		return "", libError.ErrTokenInvalidJWTToken.Err()
	}

	return claims["sub"], nil
}

func (m *AccessRefreshTokenManager) GetAccessTokenTTL() time.Duration {
	return m.accessTTL
}

func (m *AccessRefreshTokenManager) GetRefreshTokenTTL() time.Duration {
	return m.refreshTTL
}

func NewBase64TokenManager(deps Base64TokenDependencies) (*Base64TokenManager, error) {
	if deps.TTL < 0 {
		return nil, libError.ErrTokenTTLMustBeNotNegative.Err()
	}

	m, err := newTokenManager(dependencies{SigningKey: deps.SigningKey, Issuer: deps.Issuer})
	if err != nil {
		return nil, err
	}

	return &Base64TokenManager{
		manager: m,
		ttl:     deps.TTL,
	}, nil
}

func (m *Base64TokenManager) NewBase64Token(subject interface{}, ttl ...time.Duration) (string, error) {
	// If no TTL is provided, use the default TTL
	baseTTL := m.ttl

	// If a TTL is provided, use that instead
	if len(ttl) == 1 {
		if ttl[0] < 0 {
			return "", libError.ErrTokenTTLMustBeNotNegative.Err()
		}
		baseTTL = ttl[0]
	}

	strToken, err := m.manager.newToken(subject, baseTTL)
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to create token").Err()
	}

	bs64Token := base64.StdEncoding.EncodeToString([]byte(strToken))

	return bs64Token, nil
}

func (m *Base64TokenManager) ParseBase64Token(base64Token string) (interface{}, error) {
	// Decode the base64 token
	Token, err := base64.StdEncoding.DecodeString(base64Token)
	if err != nil {
		return nil, libError.ErrToken.WithError(err).WithMessage("Failed to decode base64 token").Err()
	}

	// Parse the token
	token, err := m.manager.parseToken(string(Token))
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to parse token").Err()
	}

	// Check if the token is valid
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", libError.ErrTokenInvalidJWTToken.Err()
	}

	return claims["sub"], nil
}

func newTokenManager(deps dependencies) (*manager, error) {
	if deps.SigningKey == "" {
		return nil, libError.ErrTokenEmptySignature.Err()
	}

	if deps.Issuer == "" {
		return nil, libError.ErrTokenEmptyIssuer.Err()
	}

	return &manager{signingKey: deps.SigningKey, issuer: deps.Issuer}, nil
}

func (m *manager) newToken(subject interface{}, tokenTTL time.Duration, tokenType ...int8) (string, error) {
	tokenClaims := jwt.MapClaims{}
	// Set the token claims
	// Set the token issuer
	tokenClaims["iss"] = m.issuer
	// Set the token subject
	tokenClaims["sub"] = subject
	// Set the token issued at time
	tokenClaims["iat"] = time.Now().Unix()
	// Set the token expiration time
	if tokenTTL > 0 {
		tokenClaims["exp"] = time.Now().Add(tokenTTL).Unix()
	}
	// Set token id
	tokenClaims["jti"] = uuid.New()

	// If a token type is provided, add it to the token
	if len(tokenType) > 0 {
		tokenClaims["token"] = tokenType[0]
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)

	signed, err := token.SignedString([]byte(m.signingKey))
	if err != nil {
		return "", libError.ErrToken.WithError(err).WithMessage("Failed to sign token").Err()
	}

	return signed, nil
}

func (m *manager) parseToken(Token string) (*jwt.Token, error) {
	return jwt.Parse(Token, func(token *jwt.Token) (i interface{}, err error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, libError.ErrToken.WithMessage("Unexpected signing method").WithContext("method", token.Header["alg"]).Err()
		}

		return []byte(m.signingKey), nil
	})
}
