package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// testProxy creates a Proxy with a test key (no OIDC discovery).
func testProxy(t *testing.T) *Proxy {
	t.Helper()
	key := deriveKey("test-secret-key-32-chars-minimum")
	return &Proxy{
		key:      key,
		tokenURL: "http://oidc.example.com/token",
		oauth2Config: oauth2.Config{
			ClientID:     "hugr-client",
			ClientSecret: "hugr-secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "http://oidc.example.com/authorize",
				TokenURL: "http://oidc.example.com/token",
			},
			Scopes: []string{"openid", "profile", "email"},
		},
	}
}

func TestHandleMetadata(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	req := httptest.NewRequest("GET", "https://hugr.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var metadata map[string]any
	if err := json.NewDecoder(w.Body).Decode(&metadata); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if metadata["issuer"] != "https://hugr.example.com" {
		t.Fatalf("unexpected issuer: %v", metadata["issuer"])
	}
	if metadata["authorization_endpoint"] != "https://hugr.example.com/oauth/authorize" {
		t.Fatalf("unexpected authorization_endpoint: %v", metadata["authorization_endpoint"])
	}
	if metadata["token_endpoint"] != "https://hugr.example.com/oauth/token" {
		t.Fatalf("unexpected token_endpoint: %v", metadata["token_endpoint"])
	}
	if metadata["registration_endpoint"] != "https://hugr.example.com/oauth/register" {
		t.Fatalf("unexpected registration_endpoint: %v", metadata["registration_endpoint"])
	}
}

func TestHandleAuthorize_Success(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {"test-client"},
		"redirect_uri":          {"http://localhost:12345/callback"},
		"state":                 {"client-state-123"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}

	req := httptest.NewRequest("GET", "https://hugr.example.com/oauth/authorize?"+params.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	if !strings.HasPrefix(location, "http://oidc.example.com/authorize") {
		t.Fatalf("expected redirect to OIDC provider, got: %s", location)
	}

	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if redirectURL.Query().Get("client_id") != "hugr-client" {
		t.Fatalf("expected hugr-client client_id, got: %s", redirectURL.Query().Get("client_id"))
	}
	if redirectURL.Query().Get("state") == "" {
		t.Fatal("expected encrypted state in redirect")
	}
}

func TestHandleAuthorize_MissingParams(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	req := httptest.NewRequest("GET", "https://hugr.example.com/oauth/authorize?response_type=code", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAuthorize_InvalidRedirectURI(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {"test"},
		"redirect_uri":          {"http://evil.com/callback"},
		"state":                 {"s"},
		"code_challenge":        {"c"},
		"code_challenge_method": {"S256"},
	}

	req := httptest.NewRequest("GET", "https://hugr.example.com/oauth/authorize?"+params.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-localhost non-HTTPS redirect, got %d", w.Code)
	}
}

func TestHandleToken_AuthorizationCode_Success(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	// Create a valid encrypted auth code
	authCode := AuthCodePayload{
		AccessToken:   "oidc-access-token",
		IDToken:       "oidc-id-token",
		RefreshToken:  "oidc-refresh-token",
		TokenType:     "Bearer",
		ExpiresIn:     3600,
		CodeChallenge: challenge,
		ClientID:      "test-client",
		RedirectURI:   "http://localhost:12345/callback",
	}
	encryptedCode, err := encryptAuthCode(p.key, authCode)
	if err != nil {
		t.Fatalf("encrypt auth code: %v", err)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {encryptedCode},
		"redirect_uri":  {"http://localhost:12345/callback"},
		"client_id":     {"test-client"},
		"code_verifier": {verifier},
	}

	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["access_token"] != "oidc-access-token" {
		t.Fatalf("unexpected access_token: %v", resp["access_token"])
	}
	if resp["id_token"] != "oidc-id-token" {
		t.Fatalf("unexpected id_token: %v", resp["id_token"])
	}
	if resp["refresh_token"] != "oidc-refresh-token" {
		t.Fatalf("unexpected refresh_token: %v", resp["refresh_token"])
	}
}

func TestHandleToken_PKCEMismatch(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	authCode := AuthCodePayload{
		AccessToken:   "token",
		TokenType:     "Bearer",
		CodeChallenge: "correct-challenge",
		ClientID:      "client",
		RedirectURI:   "http://localhost:1234/cb",
	}
	encryptedCode, _ := encryptAuthCode(p.key, authCode)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {encryptedCode},
		"redirect_uri":  {"http://localhost:1234/cb"},
		"client_id":     {"client"},
		"code_verifier": {"wrong-verifier"},
	}

	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for PKCE mismatch, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid_grant" {
		t.Fatalf("expected invalid_grant error, got: %s", resp["error"])
	}
}

func TestHandleToken_ExpiredCode(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	// Manually create an expired auth code
	authCode := AuthCodePayload{
		AccessToken:   "token",
		TokenType:     "Bearer",
		CodeChallenge: "challenge",
		ClientID:      "client",
		RedirectURI:   "http://localhost:1234/cb",
		Timestamp:     time.Now().Add(-2 * time.Minute).Unix(), // expired
	}
	encryptedCode, _ := encrypt(p.key, authCode)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {encryptedCode},
		"redirect_uri":  {"http://localhost:1234/cb"},
		"client_id":     {"client"},
		"code_verifier": {"verifier"},
	}

	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for expired code, got %d", w.Code)
	}
}

func TestHandleToken_ClientIDMismatch(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	authCode := AuthCodePayload{
		AccessToken:   "token",
		TokenType:     "Bearer",
		CodeChallenge: "challenge",
		ClientID:      "correct-client",
		RedirectURI:   "http://localhost:1234/cb",
	}
	encryptedCode, _ := encryptAuthCode(p.key, authCode)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {encryptedCode},
		"redirect_uri":  {"http://localhost:1234/cb"},
		"client_id":     {"wrong-client"},
		"code_verifier": {"verifier"},
	}

	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for client_id mismatch, got %d", w.Code)
	}
}

func TestHandleToken_TamperedCode(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"this-is-not-a-valid-encrypted-code"},
		"redirect_uri":  {"http://localhost:1234/cb"},
		"client_id":     {"client"},
		"code_verifier": {"verifier"},
	}

	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for tampered code, got %d", w.Code)
	}
}

func TestHandleRegister_Success(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	body := `{"redirect_uris": ["http://localhost:12345/callback"], "client_name": "Test Client"}`
	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	clientID, ok := resp["client_id"].(string)
	if !ok || !strings.HasPrefix(clientID, "hugr_mcp_") {
		t.Fatalf("unexpected client_id: %v", resp["client_id"])
	}
}

func TestHandleRegister_InvalidRedirectURI(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	body := `{"redirect_uris": ["http://evil.com/callback"]}`
	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-localhost redirect, got %d", w.Code)
	}
}

func TestHandleRegister_MissingRedirectURIs(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	body := `{"client_name": "No URIs"}`
	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing redirect_uris, got %d", w.Code)
	}
}

func TestHandleRegister_HTTPSRedirectURI(t *testing.T) {
	p := testProxy(t)
	mux := http.NewServeMux()
	p.RegisterHandlers(mux)

	body := `{"redirect_uris": ["https://app.example.com/callback"]}`
	req := httptest.NewRequest("POST", "https://hugr.example.com/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for HTTPS redirect, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIsValidRedirectURI(t *testing.T) {
	tests := []struct {
		uri  string
		want bool
	}{
		{"http://localhost:12345/callback", true},
		{"http://localhost/callback", true},
		{"http://127.0.0.1:8080/cb", true},
		{"https://app.example.com/callback", true},
		{"http://evil.com/callback", false},
		{"http://192.168.1.1:8080/cb", false},
		{"ftp://localhost/cb", true}, // localhost is always allowed
		{"not-a-url", false},
	}

	for _, tt := range tests {
		got := isValidRedirectURI(tt.uri)
		if got != tt.want {
			t.Errorf("isValidRedirectURI(%q) = %v, want %v", tt.uri, got, tt.want)
		}
	}
}

