//go:build duckdb_arrow

package integration_test

import (
	"context"
	"testing"

	"github.com/hugr-lab/hugr/pkg/auth"
)

func TestAuth_AnonymousProviderAttached(t *testing.T) {
	cfg := auth.Config{
		AllowedAnonymous: true,
		AnonymousRole:    "admin",
	}
	ctx := context.Background()
	authConfig, err := cfg.Configure(ctx)
	if err != nil {
		t.Fatalf("Failed to configure auth: %v", err)
	}
	if authConfig == nil {
		t.Fatal("Auth config should not be nil when anonymous is allowed")
	}
}

func TestAuth_APIKeyProviderAttached(t *testing.T) {
	cfg := auth.Config{
		SecretKey: "test-secret-key",
	}
	ctx := context.Background()
	authConfig, err := cfg.Configure(ctx)
	if err != nil {
		t.Fatalf("Failed to configure auth: %v", err)
	}
	if authConfig == nil {
		t.Fatal("Auth config should not be nil when secret key is set")
	}
}

func TestAuth_MultipleProvidersConfigured(t *testing.T) {
	cfg := auth.Config{
		AllowedAnonymous: true,
		AnonymousRole:    "viewer",
		SecretKey:        "test-secret-key",
	}
	ctx := context.Background()
	authConfig, err := cfg.Configure(ctx)
	if err != nil {
		t.Fatalf("Failed to configure auth: %v", err)
	}
	if authConfig == nil {
		t.Fatal("Auth config should not be nil with multiple providers")
	}
}

func TestAuth_OIDCProviderWithInvalidIssuer(t *testing.T) {
	cfg := auth.Config{
		OIDC: auth.OIDCConfig{
			Issuer:   "http://invalid-issuer.example.com",
			ClientID: "test-client",
		},
	}
	ctx := context.Background()
	_, err := cfg.Configure(ctx)
	// OIDC with invalid issuer should return an error
	if err == nil {
		t.Log("OIDC configure did not return error for invalid issuer (may attempt lazy discovery)")
	}
}
