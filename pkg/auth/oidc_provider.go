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
	Issuer      string        `json:"issuer" yaml:"issuer"`
	ClientID    string        `json:"client_id" yaml:"client_id"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
	TLSInsecure bool          `json:"tls_insecure" yaml:"tls_insecure"`
	CookieName  string        `json:"cookie_name" yaml:"cookie_name"`

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
	if c.ScopeRolePrefix == "" {
		c.ScopeRolePrefix = "hugr:"
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
		return nil, nil
	}

	idToken, err := p.verifier.Verify(r.Context(), token)
	var oidcErr *oidc.TokenExpiredError
	if errors.As(err, &oidcErr) {
		return nil, auth.ErrTokenExpired
	}
	if err != nil {
		return nil, auth.ErrForbidden
	}

	claims := jwt.MapClaims{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, auth.ErrForbidden
	}

	role, _ := claims[p.c.Claims.Role].(string)
	userId, _ := claims[p.c.Claims.UserId].(string)
	userName, _ := claims[p.c.Claims.UserName].(string)

	// check scopes if role is empty
	if role == "" {
		scopes, ok := claims["scopes"].([]any)
		if ok {
			for _, scope := range scopes {
				if s, ok := scope.(string); ok {
					if strings.HasPrefix(s, p.c.ScopeRolePrefix) {
						if role == "" || strings.HasSuffix(s, role) {
							role = strings.TrimPrefix(s, p.c.ScopeRolePrefix)
							break
						}
					}
				}
			}
		}
	}

	return &auth.AuthInfo{
		Role:         role,
		UserId:       userId,
		UserName:     userName,
		AuthType:     p.Type(),
		AuthProvider: p.Name(),
		Token:        token,
	}, nil
}
