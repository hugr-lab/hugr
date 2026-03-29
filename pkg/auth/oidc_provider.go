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
		return nil, nil
	}

	idToken, err := p.verifier.Verify(r.Context(), token)
	if _, ok := errors.AsType[*oidc.TokenExpiredError](err); ok {
		return nil, auth.ErrTokenExpired
	}
	if err != nil {
		return nil, auth.ErrForbidden
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
	}, nil
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
