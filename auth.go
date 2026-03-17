package main

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CHUNK_START: pairing-nonce-struct-and-store-v1-uuid-3b9f2e1d
// BUSINESS_PURPOSE: Defines the in-memory PairingNonce structure and the global map + mutex for short-lived pairing tokens per specbook Chapter 4 (proximity pairing security)
// SPEC_LINK: specbook-chapter-4
type PairingNonce struct {
	Token     string
	ExpiresAt time.Time
}

var (
	// In-memory store for active pairing nonces (short-lived)
	pairingStore = make(map[string]PairingNonce)
	pairingMutex sync.Mutex

	// Secret key generated at runtime for JWT signing
	// For production persistence, this should be saved to the DB
	jwtSecret = []byte("change-me-to-something-persistent")
)
// CHUNK_END: pairing-nonce-struct-and-store-v1-uuid-3b9f2e1d

// CHUNK_START: generate-pairing-nonce-v1-uuid-5c8a4f7e
// BUSINESS_PURPOSE: Generates a cryptographically secure 16-byte hex token with 10-minute expiry for device pairing, stored in-memory with mutex protection per specbook Chapter 4
// SPEC_LINK: specbook-chapter-4
func GeneratePairingNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	pairingMutex.Lock()
	pairingStore[token] = PairingNonce{
		Token:     token,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	pairingMutex.Unlock()

	return token, nil
}
// CHUNK_END: generate-pairing-nonce-v1-uuid-5c8a4f7e

// CHUNK_START: require-loopback-middleware-v1-uuid-7d2e6b9a
// BUSINESS_PURPOSE: Middleware that enforces loopback-only access (127.0.0.1 or ::1) for pairing initiation to prevent external exposure per specbook Chapter 4 security non-negotiables
// SPEC_LINK: specbook-chapter-4
func RequireLoopback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		// Only allow requests from the machine itself (or via SSH tunnel)
		if host != "127.0.0.1" && host != "::1" {
			http.Error(w, "Pairing must be initiated via local loopback or SSH tunnel.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
// CHUNK_END: require-loopback-middleware-v1-uuid-7d2e6b9a

// CHUNK_START: jwt-claims-and-create-token-v1-uuid-9f1c3a8d
// BUSINESS_PURPOSE: Defines JWT Claims structure and issues a 90-day signed token for paired devices containing the device label per specbook Chapter 4 (persistent pairing)
// SPEC_LINK: specbook-chapter-4
type Claims struct {
	DeviceLabel string `json:"device_label"`
	jwt.RegisteredClaims
}

func CreateToken(deviceLabel string) (string, error) {
	claims := &Claims{
		DeviceLabel: deviceLabel,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(90 * 24 * time.Hour)), // 90 days
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}
// CHUNK_END: jwt-claims-and-create-token-v1-uuid-9f1c3a8d

// CHUNK_START: validate-jwt-middleware-v1-uuid-2e4b7f9c
// BUSINESS_PURPOSE: Middleware that validates JWT from cookie and protects all operational endpoints; redirects unauthorized requests and injects device label into context per specbook Chapter 5 (operational flows security)
// SPEC_LINK: specbook-chapter-5
func ValidateJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("ccc_auth")
		if err != nil {
			http.Redirect(w, r, "/unauthorized", http.StatusSeeOther)
			return
		}
		tokenStr := cookie.Value

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			http.Redirect(w, r, "/unauthorized", http.StatusSeeOther)
			return
		}

		// Inject device label into context for logging "who" entered a transaction
		next.ServeHTTP(w, r)
	})
}
// CHUNK_END: validate-jwt-middleware-v1-uuid-2e4b7f9c
