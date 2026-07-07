package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang-jwt/jwt/v5/request"
	"github.com/hugr-lab/query-engine/pkg/auth"
)

type OIDCConfig struct {
	Issuer       string        `json:"issuer" yaml:"issuer"`
	ClientID     string        `json:"client_id" yaml:"client_id"`
	ClientSecret string        `json:"-" yaml:"client_secret"`
	Timeout      time.Duration `json:"timeout" yaml:"timeout"`
	TLSInsecure  bool          `json:"tls_insecure" yaml:"tls_insecure"`
	CookieName   string        `json:"cookie_name" yaml:"cookie_name"`
	Scopes       string        `json:"scopes" yaml:"scopes"`
	RedirectURL  string        `json:"redirect_url" yaml:"redirect_url"`

	ScopeRolePrefix string     `json:"scope_role_prefix" yaml:"scope_role_prefix"`
	Claims          OIDCClaims `json:"claims" yaml:"claims"`
}

type OIDCClaims = auth.UserAuthInfoConfig

type OIDCProvider struct {
	c         OIDCConfig
	verifier  verifier
	extractor request.Extractor
}

type verifier interface {
	Verify(ctx context.Context, token string) (*oidc.IDToken, error)
}

func NewOIDCProvider(ctx context.Context, c OIDCConfig) (*OIDCProvider, error) {
	if c.Issuer == "" {
		return nil, errors.New("OIDC Issuer is required")
	}
	hc := &http.Client{
		Timeout: c.Timeout,
	}
	if c.TLSInsecure {
		hc.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, hc), c.Issuer)
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID:          c.ClientID,
		SkipClientIDCheck: true,
	})
	extractor := request.OAuth2Extractor
	if c.CookieName != "" {
		extractor = &request.MultiExtractor{
			request.OAuth2Extractor,
			auth.CookieExtractor(c.CookieName),
		}
	}

	if c.Claims.Role == "" {
		c.Claims.Role = "x-hugr-role"
	}
	if c.Claims.UserId == "" {
		c.Claims.UserId = "sub"
	}
	if c.Claims.UserName == "" {
		c.Claims.UserName = "name"
	}
	return &OIDCProvider{
		c:         c,
		verifier:  verifier,
		extractor: extractor,
	}, nil
}

func (p *OIDCProvider) Name() string {
	return "oidc"
}

func (p *OIDCProvider) Type() string {
	return "oidc"
}

func (p *OIDCProvider) Authenticate(r *http.Request) (*auth.AuthInfo, error) {
	token, err := p.extractor.ExtractToken(r)
	if errors.Is(err, request.ErrNoTokenInRequest) {
		return nil, auth.ErrSkipAuth
	}
	if err != nil {
		return nil, err
	}
	if token == "" {
		// An empty credential is treated as "no token" so it does not block the
		// chain; returning (nil, nil) here would bypass the middleware's
		// rejected-token guard and could downgrade to anonymous access.
		return nil, auth.ErrSkipAuth
	}

	idToken, err := p.verifier.Verify(r.Context(), token)
	if _, ok := errors.AsType[*oidc.TokenExpiredError](err); ok {
		return nil, auth.ErrTokenExpired
	}
	if err != nil {
		// Verification failed. If the token was issued by this provider's issuer
		// it is genuinely our token but invalid (bad signature/audience/nbf, or
		// the IdP is unreachable) — a hard failure. Otherwise it was issued for a
		// different provider, so let the middleware try the next one (a token no
		// provider accepts still ends in 401, never anonymous).
		if p.tokenIssuedHere(token) {
			return nil, auth.ErrForbidden
		}
		return nil, auth.ErrInvalidKeyType
	}

	claims := jwt.MapClaims{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, auth.ErrForbidden
	}

	role := claimString(claims, p.c.Claims.Role, p.c.ScopeRolePrefix)
	userId := claimString(claims, p.c.Claims.UserId, "")
	userName := claimString(claims, p.c.Claims.UserName, "")

	// check scopes if role is empty
	if role == "" {
		role = claimString(claims, "scopes", p.c.ScopeRolePrefix)
	}

	if role == "" {
		return nil, auth.ErrForbidden
	}

	return &auth.AuthInfo{
		Role:         role,
		UserId:       userId,
		UserName:     userName,
		AuthType:     p.Type(),
		AuthProvider: p.Name(),
		Token:        token,
		Claims:       auth.ScalarClaims(claims),
	}, nil
}

// tokenIssuedHere reports whether the token's `iss` claim matches this
// provider's configured issuer. It parses the JWT WITHOUT verifying the
// signature — used only to classify a verification failure as a hard error
// (our issuer's token, genuinely invalid) versus a fallthrough to the next
// provider (a different issuer's token). A non-JWT or different/empty issuer is
// treated as "not ours".
func (p *OIDCProvider) tokenIssuedHere(token string) bool {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return false
	}
	iss, _ := claims["iss"].(string)
	return iss == p.c.Issuer
}

// claimString extracts a string value from a claim, if prefix is provided, it return only unprefixed value with it.
// claims can be in different formats, for example:
// "scopes": ["hugr:admin", "app:user"] -> for prefix "hugr:" it will return "admin", for prefix "app:" it will return "user", for prefix "" it will return "hugr:admin" (first match)
// "roles": "hugr:user" -> for prefix "hugr:" it will return "user", for prefix "" it will return "hugr:user", for prefix "app:" it will return "" (no match)
// "x-hugr-role": "admin"
func claimString(claims jwt.MapClaims, key, prefix string) string {
	if len(claims) == 0 {
		return ""
	}
	v, ok := claims[key]
	if !ok {
		return ""
	}
	var s string
	switch val := v.(type) {
	case string:
		if after, ok := strings.CutPrefix(val, prefix); ok {
			s = after
		}
	case []any:
		for _, item := range val {
			str, ok := item.(string)
			if !ok {
				continue
			}
			if after, ok := strings.CutPrefix(str, prefix); ok {
				s = after
				break
			}
		}
	case []string:
		for _, str := range val {
			if after, ok := strings.CutPrefix(str, prefix); ok {
				s = after
				break
			}
		}
	}
	return s
}
