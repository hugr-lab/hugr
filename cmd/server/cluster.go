package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/hugr-lab/hugr/pkg/auth"
	"github.com/hugr-lab/hugr/pkg/cluster"
	hugr "github.com/hugr-lab/query-engine"
	hauth "github.com/hugr-lab/query-engine/pkg/auth"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
)

type ClusterConfig struct {
	ManagementUrl string
	Secret        string
	NodeName      string
	NodeUrl       string
	Timeout       time.Duration
}

func RegisterNode(ctx context.Context, c ClusterConfig, lc Config) (hugr.Config, error) {
	if c.ManagementUrl == "" {
		return hugr.Config{}, errors.New("cluster URL is required")
	}
	u, err := url.Parse(c.ManagementUrl)
	if err != nil {
		return hugr.Config{}, errors.New("invalid cluster URL")
	}
	params := url.Values{
		"url":     {c.NodeUrl},
		"name":    {c.NodeName},
		"version": {Version},
	}
	u.RawQuery = params.Encode()
	u.Path = "/node"
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if c.Secret == "" {
		return hugr.Config{}, errors.New("cluster secret is required")
	}

	req.Header.Set("x-hugr-secret", c.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return hugr.Config{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return hugr.Config{}, errors.New("failed to register node: " + resp.Status)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		return hugr.Config{}, errors.New("invalid response format")
	}
	var nc cluster.NodeCommonConfig
	err = json.NewDecoder(resp.Body).Decode(&nc)
	if err != nil {
		return hugr.Config{}, errors.New("failed to decode response: " + err.Error())
	}

	// auth
	hc := hugr.Config{
		AdminUI:            lc.EnableAdminUI,
		AdminUIFetchPath:   nc.AdminUIFetchPath,
		Debug:              nc.DebugMode,
		AllowParallel:      lc.AllowParallel,
		MaxParallelQueries: lc.MaxParallelQueries,
		MaxDepth:           lc.MaxDepthInTypes,
		DB:                 lc.DB,
		CoreDB:             coredb.New(nc.CoreDB),
		Cache:              lc.Cache,
	}

	// auth
	hc.Auth = &hauth.Config{
		RedirectLoginPaths: []string{"/admin"},
		LoginUrl:           nc.Auth.LoginUrl,
		RedirectUrl:        nc.Auth.RedirectUrl,
		DBApiKeysEnabled:   nc.Auth.ManagedAPIKeysEnabled,
	}

	// 0. secret
	if c.Secret != "" {
		hc.Auth.Providers = append(hc.Auth.Providers,
			hauth.NewApiKey("x-hugr-secret", hauth.ApiKeyConfig{
				Key:         c.Secret,
				Header:      "x-hugr-secret",
				DefaultRole: "admin",
				Headers: hauth.UserAuthInfoConfig{
					Role:     "x-hugr-role",
					UserId:   "x-hugr-user-id",
					UserName: "x-hugr-user-name",
				},
			}),
		)
	}

	// 1. oidc
	if nc.Auth.OIDC.Issuer != "" {
		oidc, err := auth.NewOIDCProvider(ctx, nc.Auth.OIDC)
		if err != nil {
			return hugr.Config{}, err
		}
		hc.Auth.Providers = append(hc.Auth.Providers, oidc)
	}

	// 2. api keys
	for name, pc := range nc.Auth.APIKeys {
		if name == "x-hugr-secret" {
			continue
		}
		apiKey := hauth.NewApiKey(name, pc)
		if err != nil {
			return hugr.Config{}, err
		}
		hc.Auth.Providers = append(hc.Auth.Providers, apiKey)
	}

	// 3. jwt
	for name, pc := range nc.Auth.JWT {
		if name == "x-hugr-secret" {
			continue
		}
		jwtProvider, err := hauth.NewJwt(&pc)
		if err != nil {
			return hugr.Config{}, err
		}
		hc.Auth.Providers = append(hc.Auth.Providers, jwtProvider)
	}

	// 4. anonymous
	if nc.Auth.Anonymous.Allowed {
		hc.Auth.Providers = append(hc.Auth.Providers,
			hauth.NewAnonymous(hauth.AnonymousConfig{
				Allowed: true,
				Role:    nc.Auth.Anonymous.Role,
			}),
		)
	}

	return hc, nil
}

func UnregisterNode(ctx context.Context, c ClusterConfig) error {
	if c.ManagementUrl == "" {
		return errors.New("cluster URL is required")
	}
	u, err := url.Parse(c.ManagementUrl)
	if err != nil {
		return errors.New("invalid cluster URL")
	}
	params := url.Values{
		"name": {c.NodeName},
	}
	u.RawQuery = params.Encode()
	u.Path = "/node"
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if c.Secret == "" {
		return errors.New("cluster secret is required")
	}

	req.Header.Set("x-hugr-secret", c.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("failed to unregister node: " + resp.Status)
	}
	return nil
}
