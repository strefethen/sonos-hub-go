package applemusic

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager handles Apple Music developer token generation and caching.
// Tokens are JWTs signed with ES256 using the private key from Apple Developer.
type TokenManager struct {
	teamID     string
	keyID      string
	privateKey *ecdsa.PrivateKey
	expiry     time.Duration

	mu          sync.RWMutex
	cachedToken string
	tokenExpiry time.Time
}

// TokenManagerConfig holds configuration for creating a TokenManager.
type TokenManagerConfig struct {
	TeamID         string        // Apple Developer Team ID
	KeyID          string        // Apple Music Key ID
	PrivateKeyPath string        // Path to .p8 private key file
	Expiry         time.Duration // Token TTL (max 6 months, recommended 24h)
}

// NewTokenManager creates a new TokenManager from configuration.
// Returns an error if the private key cannot be loaded or parsed.
func NewTokenManager(cfg TokenManagerConfig) (*TokenManager, error) {
	if cfg.TeamID == "" {
		return nil, fmt.Errorf("team ID is required")
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("key ID is required")
	}
	if cfg.PrivateKeyPath == "" {
		return nil, fmt.Errorf("private key path is required")
	}

	// Load and parse the private key
	privateKey, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load private key: %w", err)
	}

	// Default to 24 hour expiry if not specified
	expiry := cfg.Expiry
	if expiry == 0 {
		expiry = 24 * time.Hour
	}

	return &TokenManager{
		teamID:     cfg.TeamID,
		keyID:      cfg.KeyID,
		privateKey: privateKey,
		expiry:     expiry,
	}, nil
}

// GetToken returns a valid developer token, generating a new one if needed.
// The token is cached and reused until it's within 5 minutes of expiration.
func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.RLock()
	// Check if we have a valid cached token (with 5 minute buffer)
	if tm.cachedToken != "" && time.Now().Add(5*time.Minute).Before(tm.tokenExpiry) {
		token := tm.cachedToken
		tm.mu.RUnlock()
		return token, nil
	}
	tm.mu.RUnlock()

	// Need to generate a new token
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring write lock
	if tm.cachedToken != "" && time.Now().Add(5*time.Minute).Before(tm.tokenExpiry) {
		return tm.cachedToken, nil
	}

	// Generate new token
	token, expiry, err := tm.generateToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	tm.cachedToken = token
	tm.tokenExpiry = expiry

	return token, nil
}

// generateToken creates a new JWT for Apple Music API authentication.
func (tm *TokenManager) generateToken() (string, time.Time, error) {
	now := time.Now()
	expiry := now.Add(tm.expiry)

	// Create claims
	claims := jwt.MapClaims{
		"iss": tm.teamID,
		"iat": now.Unix(),
		"exp": expiry.Unix(),
	}

	// Create token with ES256 algorithm
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)

	// Set key ID in header
	token.Header["kid"] = tm.keyID

	// Sign the token
	signedToken, err := token.SignedString(tm.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}

	return signedToken, expiry, nil
}

// loadPrivateKey reads and parses an ECDSA private key from a .p8 file.
// Apple provides keys in PKCS#8 PEM format.
func loadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Decode PEM block
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in file")
	}

	// Parse PKCS#8 private key
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8 key: %w", err)
	}

	// Assert it's an ECDSA key
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not ECDSA (got %T)", key)
	}

	return ecdsaKey, nil
}
