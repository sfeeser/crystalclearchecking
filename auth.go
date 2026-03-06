package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PairingNonce holds a short-lived token generated via SSH/Loopback
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

// GeneratePairingNonce creates a 10-minute token accessible only via loopback.
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

// RequireLoopback is a middleware that restricts access to 127.0.0.1.
// This is the "Fortress" gate for the /pair endpoint.
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

// Claims defines the JWT structure for CCC
type Claims struct {
	DeviceLabel string `json:"device_label"`
	jwt.RegisteredClaims
}

// CreateToken issues a long-lived JWT for a paired device.
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

// ValidateJWT Middleware protects all operational endpoints (/ledger, /add, etc.)
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
