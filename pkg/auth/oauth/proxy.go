package oauth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	stateTTL    = 5 * time.Minute
	authCodeTTL = 60 * time.Second
)

// Config holds the configuration for the OAuth proxy.
type Config struct {
	// OIDC provider settings
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       string
	RedirectURL  string // optional override for callback URL derivation
	TLSInsecure  bool

	// Encryption key for state and auth codes
	SecretKey string
}

// Proxy is a stateless OAuth 2.1 proxy that delegates authentication
// to an external OIDC provider. It encrypts all transient state into
// request/response parameters using AES-GCM.
type Proxy struct {
	oauth2Config oauth2.Config
	oidcProvider *oidc.Provider
	key          []byte
	redirectURL  string // optional override
	tokenURL     string // OIDC provider's token endpoint (for refresh proxy)
}

// NewProxy creates a new OAuth proxy by performing OIDC discovery.
func NewProxy(ctx context.Context, cfg Config) (*Proxy, error) {
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("SECRET_KEY is required for MCP OAuth proxy")
	}

	oidcCtx := ctx
	if cfg.TLSInsecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		hc := &http.Client{Transport: tr}
		oidcCtx = oidc.ClientContext(ctx, hc)
	}

	provider, err := oidc.NewProvider(oidcCtx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	scopes := []string{oidc.ScopeOpenID}
	if cfg.Scopes != "" {
		scopes = strings.Fields(cfg.Scopes)
	}

	// Extract token endpoint from provider for refresh proxy
	var providerClaims struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := provider.Claims(&providerClaims); err != nil {
		return nil, fmt.Errorf("oidc provider claims: %w", err)
	}

	p := &Proxy{
		oidcProvider: provider,
		key:          deriveKey(cfg.SecretKey),
		redirectURL:  cfg.RedirectURL,
		tokenURL:     providerClaims.TokenEndpoint,
		oauth2Config: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       scopes,
		},
	}

	return p, nil
}

// RegisterHandlers registers all OAuth proxy endpoints on the mux.
func (p *Proxy) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", p.handleMetadata)
	mux.HandleFunc("GET /oauth/authorize", p.handleAuthorize)
	mux.HandleFunc("GET /oauth/callback", p.handleCallback)
	mux.HandleFunc("POST /oauth/token", p.handleToken)
	mux.HandleFunc("POST /oauth/register", p.handleRegister)
}

// callbackURL returns the OAuth callback URL for Hugr.
// Uses the configured RedirectURL if set, otherwise derives from the request Host header.
func (p *Proxy) callbackURL(r *http.Request) string {
	if p.redirectURL != "" {
		return p.redirectURL
	}
	return requestScheme(r) + "://" + r.Host + "/oauth/callback"
}

// baseURL returns the base URL of the Hugr server derived from the request.
func baseURL(r *http.Request) string {
	return requestScheme(r) + "://" + r.Host
}

// requestScheme detects the scheme from TLS state, X-Forwarded-Proto header,
// or defaults to http for plain connections.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		return fwd
	}
	return "http"
}

// handleMetadata serves the OAuth 2.1 Authorization Server Metadata.
func (p *Proxy) handleMetadata(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	metadata := map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// handleAuthorize starts the OAuth flow by encrypting the MCP client's
// session into the state parameter and redirecting to the OIDC provider.
func (p *Proxy) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	responseType := q.Get("response_type")
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")

	if responseType != "code" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "response_type must be 'code'")
		return
	}
	if clientID == "" || redirectURI == "" || state == "" || codeChallenge == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "missing required parameters")
		return
	}
	if codeChallengeMethod != "S256" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code_challenge_method must be 'S256'")
		return
	}
	if !isValidRedirectURI(redirectURI) {
		oauthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri must be localhost or HTTPS")
		return
	}

	payload := &StatePayload{
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		ClientID:      clientID,
		ClientState:   state,
	}
	encryptedState, err := encryptState(p.key, payload)
	if err != nil {
		log.Printf("oauth: encrypt state error: %v", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "failed to process request")
		return
	}

	cfg := p.oauth2Config
	cfg.RedirectURL = p.callbackURL(r)

	authURL := cfg.AuthCodeURL(encryptedState)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback receives the OIDC provider's callback, exchanges the code
// for tokens, encrypts them into a Hugr authorization code, and redirects
// back to the MCP client.
func (p *Proxy) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if errCode := q.Get("error"); errCode != "" {
		errDesc := q.Get("error_description")
		log.Printf("oauth: OIDC provider error: %s: %s", errCode, errDesc)
		oauthError(w, http.StatusBadRequest, errCode, errDesc)
		return
	}

	code := q.Get("code")
	encryptedState := q.Get("state")
	if code == "" || encryptedState == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "missing code or state")
		return
	}

	state, err := decryptState(p.key, encryptedState, stateTTL)
	if err != nil {
		log.Printf("oauth: decrypt state error: %v", err)
		oauthError(w, http.StatusBadRequest, "invalid_request", "invalid or expired state")
		return
	}

	cfg := p.oauth2Config
	cfg.RedirectURL = p.callbackURL(r)

	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("oauth: token exchange error: %v", err)
		oauthError(w, http.StatusBadGateway, "server_error", "failed to exchange authorization code with OIDC provider")
		return
	}

	idTokenRaw, _ := token.Extra("id_token").(string)

	authCode := &AuthCodePayload{
		AccessToken:   token.AccessToken,
		IDToken:       idTokenRaw,
		RefreshToken:  token.RefreshToken,
		TokenType:     token.TokenType,
		ExpiresIn:     int64(time.Until(token.Expiry).Seconds()),
		CodeChallenge: state.CodeChallenge,
		ClientID:      state.ClientID,
		RedirectURI:   state.RedirectURI,
	}

	encryptedCode, err := encryptAuthCode(p.key, authCode)
	if err != nil {
		log.Printf("oauth: encrypt auth code error: %v", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "failed to process request")
		return
	}

	redirectURL, err := url.Parse(state.RedirectURI)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
		return
	}
	rq := redirectURL.Query()
	rq.Set("code", encryptedCode)
	rq.Set("state", state.ClientState)
	redirectURL.RawQuery = rq.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// handleToken exchanges an encrypted authorization code for OIDC tokens,
// or proxies a refresh token request to the OIDC provider.
func (p *Proxy) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "malformed request body")
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		p.handleTokenAuthorizationCode(w, r)
	case "refresh_token":
		p.handleTokenRefresh(w, r)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be 'authorization_code' or 'refresh_token'")
	}
}

func (p *Proxy) handleTokenAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" || redirectURI == "" || clientID == "" || codeVerifier == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "missing required parameters")
		return
	}

	authCode, err := decryptAuthCode(p.key, code, authCodeTTL)
	if err != nil {
		log.Printf("oauth: decrypt auth code error: %v", err)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "invalid or expired authorization code")
		return
	}

	if authCode.ClientID != clientID {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	if authCode.RedirectURI != redirectURI {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}
	if !verifyCodeChallenge(codeVerifier, authCode.CodeChallenge) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	resp := map[string]any{
		"access_token": authCode.AccessToken,
		"token_type":   authCode.TokenType,
	}
	if authCode.ExpiresIn > 0 {
		resp["expires_in"] = authCode.ExpiresIn
	}
	if authCode.IDToken != "" {
		resp["id_token"] = authCode.IDToken
	}
	if authCode.RefreshToken != "" {
		resp["refresh_token"] = authCode.RefreshToken
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(resp)
}

func (p *Proxy) handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "missing refresh_token")
		return
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {p.oauth2Config.ClientID},
		"client_secret": {p.oauth2Config.ClientSecret},
	}

	resp, err := http.PostForm(p.tokenURL, data)
	if err != nil {
		log.Printf("oauth: refresh proxy error: %v", err)
		oauthError(w, http.StatusBadGateway, "server_error", "failed to contact OIDC provider")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)

	var body json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		oauthError(w, http.StatusBadGateway, "server_error", "invalid response from OIDC provider")
		return
	}
	json.NewEncoder(w).Encode(body)
}

// handleRegister implements stateless dynamic client registration (RFC 7591).
func (p *Proxy) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURIs            []string `json:"redirect_uris"`
		ClientName              string   `json:"client_name"`
		GrantTypes              []string `json:"grant_types"`
		ResponseTypes           []string `json:"response_types"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "malformed JSON body")
		return
	}

	if len(req.RedirectURIs) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "redirect_uris is required")
		return
	}
	for _, uri := range req.RedirectURIs {
		if !isValidRedirectURI(uri) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri must be localhost or HTTPS: "+uri)
			return
		}
	}

	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{"authorization_code"}
	}
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []string{"code"}
	}

	clientID := generateClientID()

	resp := map[string]any{
		"client_id":                    clientID,
		"client_id_issued_at":          time.Now().Unix(),
		"redirect_uris":               req.RedirectURIs,
		"client_name":                 req.ClientName,
		"grant_types":                 req.GrantTypes,
		"response_types":              req.ResponseTypes,
		"token_endpoint_auth_method":  "none",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// isValidRedirectURI checks that a redirect URI is localhost or HTTPS.
func isValidRedirectURI(rawURI string) bool {
	u, err := url.Parse(rawURI)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]" {
		return true
	}
	return u.Scheme == "https"
}

func generateClientID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "hugr_mcp_" + hex.EncodeToString(b)
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}
