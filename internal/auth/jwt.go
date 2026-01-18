package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/strefethen/sonos-hub-go/internal/config"
)

// TokenType describes access vs refresh tokens.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// TokenPayload represents the validated payload data.
type TokenPayload struct {
	Sub        string
	DeviceName string
	Type       TokenType
}

// TokenPair is returned for pairing and refresh flows.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresInSec int
}

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
	ErrTokenType    = errors.New("token has invalid type")
)

type tokenClaims struct {
	DeviceName string    `json:"deviceName"`
	Type       TokenType `json:"type"`
	jwt.RegisteredClaims
}

// GenerateTokenPair creates a new access and refresh token.
func GenerateTokenPair(cfg config.Config, payload TokenPayload) (TokenPair, error) {
	accessToken, err := generateToken(cfg, payload, TokenTypeAccess, cfg.JWTAccessTokenExpirySec)
	if err != nil {
		return TokenPair{}, err
	}
	refreshToken, err := generateToken(cfg, payload, TokenTypeRefresh, cfg.JWTRefreshTokenExpirySec)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresInSec: cfg.JWTAccessTokenExpirySec,
	}, nil
}

// RefreshAccessToken validates a refresh token and returns a new access token.
func RefreshAccessToken(cfg config.Config, refreshToken string) (string, int, error) {
	payload, err := VerifyToken(cfg, refreshToken)
	if err != nil {
		return "", 0, err
	}
	if payload.Type != TokenTypeRefresh {
		return "", 0, ErrTokenType
	}
	accessToken, err := generateToken(cfg, payload, TokenTypeAccess, cfg.JWTAccessTokenExpirySec)
	if err != nil {
		return "", 0, err
	}
	return accessToken, cfg.JWTAccessTokenExpirySec, nil
}

// VerifyToken parses and validates the JWT.
func VerifyToken(cfg config.Config, token string) (TokenPayload, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
		jwt.WithAudience("sonos-hub-client"),
		jwt.WithIssuer("sonos-hub"),
	)

	claims := &tokenClaims{}
	parsed, err := parser.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return TokenPayload{}, ErrTokenExpired
		}
		return TokenPayload{}, ErrTokenInvalid
	}
	if parsed == nil || !parsed.Valid {
		return TokenPayload{}, ErrTokenInvalid
	}

	payload := TokenPayload{
		Sub:        claims.Subject,
		DeviceName: claims.DeviceName,
		Type:       claims.Type,
	}
	if payload.Sub == "" || payload.DeviceName == "" {
		return TokenPayload{}, ErrTokenInvalid
	}
	if payload.Type != TokenTypeAccess && payload.Type != TokenTypeRefresh {
		return TokenPayload{}, ErrTokenInvalid
	}

	return payload, nil
}

func generateToken(cfg config.Config, payload TokenPayload, tokenType TokenType, expirySec int) (string, error) {
	now := time.Now()
	claims := tokenClaims{
		DeviceName: payload.DeviceName,
		Type:       tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   payload.Sub,
			Issuer:    "sonos-hub",
			Audience:  []string{"sonos-hub-client"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expirySec) * time.Second)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWTSecret))
}
