package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/hugr-lab/query-engine/pkg/auth"
)

type stubVerifier struct{ err error }

func (s stubVerifier) Verify(ctx context.Context, token string) (*oidc.IDToken, error) {
	return nil, s.err
}

type stubExtractor string

func (s stubExtractor) ExtractToken(*http.Request) (string, error) { return string(s), nil }

func oidcTokenWithIssuer(t *testing.T, iss string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": iss, "sub": "u"}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// A Verify failure must be classified by issuer: a token from a different
// issuer falls through to the next provider (ErrInvalidKeyType), while a token
// from this provider's own issuer is a hard failure (ErrForbidden) — a genuine
// verify error (bad signature/audience/nbf, or the IdP being unreachable) must
// not be silently rerouted. An empty credential is treated as no token.
func TestOIDCProvider_Authenticate_Classification(t *testing.T) {
	verifyErr := errors.New("verify failed")
	req := httptest.NewRequest("GET", "/", nil)

	newP := func(issuer, token string) *OIDCProvider {
		return &OIDCProvider{
			c:         OIDCConfig{Issuer: issuer},
			verifier:  stubVerifier{err: verifyErr},
			extractor: stubExtractor(token),
		}
	}

	t.Run("verify fails, foreign issuer -> ErrInvalidKeyType (fall through)", func(t *testing.T) {
		_, err := newP("https://us", oidcTokenWithIssuer(t, "https://them")).Authenticate(req)
		if !errors.Is(err, auth.ErrInvalidKeyType) {
			t.Fatalf("err = %v, want ErrInvalidKeyType", err)
		}
	})

	t.Run("verify fails, our issuer -> ErrForbidden (hard fail)", func(t *testing.T) {
		_, err := newP("https://us", oidcTokenWithIssuer(t, "https://us")).Authenticate(req)
		if !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("empty token -> ErrSkipAuth", func(t *testing.T) {
		_, err := newP("https://us", "").Authenticate(req)
		if !errors.Is(err, auth.ErrSkipAuth) {
			t.Fatalf("err = %v, want ErrSkipAuth", err)
		}
	})
}
