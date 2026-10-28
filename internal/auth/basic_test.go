package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBasicAuthFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv("MIO_AUTH_MODE", "")
	t.Setenv("MIO_AUTH_USERNAME", "")
	t.Setenv("MIO_AUTH_PASSWORD_FILE", "")

	got, err := LoadBasicAuthFromEnv()
	if err != nil {
		t.Fatalf("LoadBasicAuthFromEnv() error = %v", err)
	}
	if got != nil {
		t.Fatalf("LoadBasicAuthFromEnv() = %v, want nil when disabled", got)
	}
}

func TestLoadBasicAuthFromEnvReadsPasswordFile(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "mio_password")
	if err := os.WriteFile(passwordFile, []byte("correct horse battery staple\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIO_AUTH_MODE", "basic")
	t.Setenv("MIO_AUTH_USERNAME", "operator")
	t.Setenv("MIO_AUTH_PASSWORD_FILE", passwordFile)

	got, err := LoadBasicAuthFromEnv()
	if err != nil {
		t.Fatalf("LoadBasicAuthFromEnv() error = %v", err)
	}
	if got == nil || !got.Matches("operator", "correct horse battery staple") {
		t.Fatal("loaded authenticator did not accept configured credentials")
	}
	if got.Matches("operator", "correct horse battery staple\n") {
		t.Fatal("password file newline was not handled as a line ending")
	}
}

func TestReadPasswordFileAcceptsCRLFWithinPasswordLimit(t *testing.T) {
	password := strings.Repeat("x", maxPasswordBytes)
	passwordFile := filepath.Join(t.TempDir(), "mio_password")
	if err := os.WriteFile(passwordFile, []byte(password+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPasswordFile(passwordFile)
	if err != nil {
		t.Fatalf("ReadPasswordFile() error = %v", err)
	}
	if got != password {
		t.Fatalf("ReadPasswordFile() returned %d password bytes, want %d", len(got), len(password))
	}
}

func TestReadPasswordFileRejectsOversizedFileAfterValidMaxLengthPrefix(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "mio_password")
	contents := strings.Repeat("x", maxPasswordBytes) + "\r\nextra"
	if err := os.WriteFile(passwordFile, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadPasswordFile(passwordFile); err == nil {
		t.Fatal("ReadPasswordFile() error = nil, want oversized file to be rejected")
	}
}

func TestLoadBasicAuthFromEnvFailsClosedForIncompleteConfiguration(t *testing.T) {
	t.Setenv("MIO_AUTH_MODE", "basic")
	t.Setenv("MIO_AUTH_USERNAME", "operator")
	t.Setenv("MIO_AUTH_PASSWORD_FILE", "")

	got, err := LoadBasicAuthFromEnv()
	if err == nil {
		t.Fatal("LoadBasicAuthFromEnv() error = nil, want incomplete configuration error")
	}
	if got != nil {
		t.Fatalf("LoadBasicAuthFromEnv() = %v, want nil on error", got)
	}
}

func TestLoadBasicAuthFromEnvDoesNotExposeSecretInErrors(t *testing.T) {
	const password = "never-print-this-secret"
	t.Setenv("MIO_AUTH_MODE", "basic")
	t.Setenv("MIO_AUTH_USERNAME", "operator")
	t.Setenv("MIO_AUTH_PASSWORD_FILE", filepath.Join(t.TempDir(), password))

	_, err := LoadBasicAuthFromEnv()
	if err == nil {
		t.Fatal("LoadBasicAuthFromEnv() error = nil, want missing file error")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("error exposed password-like path content: %v", err)
	}
}

func TestBasicAuthenticatorMatchesCredentials(t *testing.T) {
	authenticator, err := NewBasicAuthenticator("operator", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !authenticator.Matches("operator", "correct horse battery staple") {
		t.Fatal("configured credentials did not match")
	}
	for _, pair := range [][2]string{{"operator", "wrong"}, {"other", "correct horse battery staple"}, {"", ""}} {
		if authenticator.Matches(pair[0], pair[1]) {
			t.Errorf("unexpected match for username/password pair %q/%q", pair[0], pair[1])
		}
	}
}

func TestChallengeReturnsNoStoreBasicChallenge(t *testing.T) {
	w := httptest.NewRecorder()
	Challenge(w)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := w.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Basic ") {
		t.Errorf("WWW-Authenticate = %q, want Basic challenge", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestNewBasicAuthenticatorRejectsInvalidUsername(t *testing.T) {
	for _, username := range []string{"", "bad:name", "bad\nname"} {
		if _, err := NewBasicAuthenticator(username, "password"); err == nil {
			t.Errorf("NewBasicAuthenticator(%q) error = nil, want validation error", username)
		}
	}
}
