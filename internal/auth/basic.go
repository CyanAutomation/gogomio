// Package auth provides HTTP authentication primitives for GoGoMio.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxPasswordBytes = 4096

// BasicAuthenticator validates one configured HTTP Basic Auth account.
// Credential digests avoid storing or exposing the configured password after
// startup while keeping per-request checks inexpensive for long-lived streams.
type BasicAuthenticator struct {
	usernameDigest [sha256.Size]byte
	passwordDigest [sha256.Size]byte
}

// LoadBasicAuthFromEnv loads optional Basic Auth configuration. Authentication
// is disabled when MIO_AUTH_MODE is unset or set to "off". If enabled, the
// username and password file must both be configured and valid.
func LoadBasicAuthFromEnv() (*BasicAuthenticator, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("MIO_AUTH_MODE")))
	username := os.Getenv("MIO_AUTH_USERNAME")
	passwordFile := os.Getenv("MIO_AUTH_PASSWORD_FILE")

	switch mode {
	case "", "off":
		if username != "" || passwordFile != "" {
			return nil, fmt.Errorf("MIO_AUTH_USERNAME and MIO_AUTH_PASSWORD_FILE require MIO_AUTH_MODE=basic")
		}
		return nil, nil
	case "basic":
	default:
		return nil, fmt.Errorf("MIO_AUTH_MODE must be empty, off, or basic")
	}

	if username == "" || passwordFile == "" {
		return nil, fmt.Errorf("MIO_AUTH_USERNAME and MIO_AUTH_PASSWORD_FILE are required when MIO_AUTH_MODE=basic")
	}
	password, err := ReadPasswordFile(passwordFile)
	if err != nil {
		return nil, err
	}
	return NewBasicAuthenticator(username, password)
}

// ReadPasswordFile reads a single-line password, allowing one final LF or CRLF
// line ending. The file size is bounded before its contents are retained.
func ReadPasswordFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("password file path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("unable to read configured password file")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPasswordBytes+3))
	if err != nil {
		return "", fmt.Errorf("unable to read configured password file")
	}
	if len(data) > maxPasswordBytes+2 {
		return "", fmt.Errorf("MIO_AUTH_PASSWORD_FILE exceeds the maximum size")
	}
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
		if len(data) > 0 && data[len(data)-1] == '\r' {
			data = data[:len(data)-1]
		}
	}
	for _, b := range data {
		if b == '\r' || b == '\n' || b == 0 {
			return "", fmt.Errorf("MIO_AUTH_PASSWORD_FILE must contain one password line")
		}
	}
	if len(data) == 0 {
		return "", fmt.Errorf("MIO_AUTH_PASSWORD_FILE must not be empty")
	}
	if len(data) > maxPasswordBytes {
		return "", fmt.Errorf("MIO_AUTH_PASSWORD_FILE exceeds the maximum size")
	}
	return string(data), nil
}

// NewBasicAuthenticator creates a verifier for one username and password.
func NewBasicAuthenticator(username, password string) (*BasicAuthenticator, error) {
	if username == "" || len(username) > 256 || strings.ContainsAny(username, ":\r\n\x00") {
		return nil, fmt.Errorf("Basic Auth username is invalid")
	}
	if password == "" || len(password) > maxPasswordBytes || strings.ContainsAny(password, "\r\n\x00") {
		return nil, fmt.Errorf("Basic Auth password is invalid")
	}
	return &BasicAuthenticator{
		usernameDigest: sha256.Sum256([]byte(username)),
		passwordDigest: sha256.Sum256([]byte(password)),
	}, nil
}

// Matches reports whether the supplied credentials match the configured account.
func (a *BasicAuthenticator) Matches(username, password string) bool {
	if a == nil || len(username) > 256 || len(password) > maxPasswordBytes {
		return false
	}
	usernameDigest := sha256.Sum256([]byte(username))
	passwordDigest := sha256.Sum256([]byte(password))
	usernameMatch := subtle.ConstantTimeCompare(usernameDigest[:], a.usernameDigest[:])
	passwordMatch := subtle.ConstantTimeCompare(passwordDigest[:], a.passwordDigest[:])
	return usernameMatch&passwordMatch == 1
}

// Challenge sends a generic HTTP Basic Auth challenge without caching it.
func Challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="GoGoMio", charset="UTF-8"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
}
