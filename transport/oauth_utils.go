package transport

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GenerateRandomString generates a cryptographically secure random string of the specified length
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	
	// Use URL-safe base64 encoding without padding
	return base64.RawURLEncoding.EncodeToString(bytes)[:length], nil
}

// GenerateCodeVerifier generates a PKCE code verifier
func GenerateCodeVerifier() (string, error) {
	// RFC 7636 specifies the code verifier should be 43-128 characters
	// We'll use 64 characters for a good balance
	return GenerateRandomString(64)
}

// GenerateCodeChallenge generates a PKCE code challenge from a verifier using S256 method
func GenerateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GenerateState generates a random state parameter for OAuth flows
func GenerateState() (string, error) {
	// 32 characters should be sufficient for state
	return GenerateRandomString(32)
}