package main

import (
	"time"

	"github.com/hugr-lab/hugr/pkg/auth"
	"github.com/hugr-lab/hugr/pkg/cluster"
	"github.com/hugr-lab/hugr/pkg/cors"
	hauth "github.com/hugr-lab/query-engine/pkg/auth"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

func init() {
	initEnvs()
}

func initEnvs() {
	_ = godotenv.Overload()
	viper.SetDefault("BIND", ":14000")
	viper.SetDefault("ADMIN_UI", true)
	viper.SetDefault("ADMIN_UI_FETCH_PATH", "")
	viper.SetDefault("DEBUG", false)
	viper.SetDefault("SECRET", "hugr")
	viper.SetDefault("TIMEOUT", 30*time.Second)
	viper.SetDefault("CHECK", 1*time.Minute)
	viper.AutomaticEnv()
}

type Config struct {
	Bind    string
	Cluster cluster.Config
	OIDC    auth.OIDCConfig
	auth    auth.Config
}

func loadConfig() Config {
	return Config{
		Bind: viper.GetString("BIND"),
		OIDC: auth.OIDCConfig{
			Issuer:          viper.GetString("OIDC_ISSUER"),
			ClientID:        viper.GetString("OIDC_CLIENT_ID"),
			Timeout:         viper.GetDuration("OIDC_TIMEOUT"),
			TLSInsecure:     viper.GetBool("OIDC_TLS_INSECURE"),
			CookieName:      viper.GetString("OIDC_COOKIE_NAME"),
			ScopeRolePrefix: viper.GetString("OIDC_SCOPE_ROLE_PREFIX"),
			Claims: hauth.UserAuthInfoConfig{
				UserName: viper.GetString("OIDC_USERNAME_CLAIM"),
				UserId:   viper.GetString("OIDC_USERID_CLAIM"),
				Role:     viper.GetString("OIDC_ROLE_CLAIM"),
			},
		},
		Cluster: cluster.Config{
			Secret:  viper.GetString("SECRET"),
			Version: Version,
			Timeout: viper.GetDuration("TIMEOUT"),
			Check:   viper.GetDuration("CHECK"),
			Node: cluster.NodeCommonConfig{
				EnableAdminUI:    viper.GetBool("ADMIN_UI"),
				AdminUIFetchPath: viper.GetString("ADMIN_UI_FETCH_PATH"),
				DebugMode:        viper.GetBool("DEBUG"),
				Cors: cors.Config{
					CorsAllowedOrigins: viper.GetStringSlice("CORS_ALLOWED_ORIGINS"),
					CorsAllowedHeaders: viper.GetStringSlice("CORS_ALLOWED_HEADERS"),
					CorsAllowedMethods: viper.GetStringSlice("CORS_ALLOWED_METHODS"),
				},
				CoreDB: coredb.Config{
					Path:       viper.GetString("CORE_DB_PATH"),
					ReadOnly:   viper.GetBool("CORE_DB_READONLY"),
					S3Endpoint: viper.GetString("CORE_DB_S3_ENDPOINT"),
					S3Region:   viper.GetString("CORE_DB_S3_REGION"),
					S3Key:      viper.GetString("CORE_DB_S3_KEY"),
					S3Secret:   viper.GetString("CORE_DB_S3_SECRET"),
					S3UseSSL:   viper.GetBool("CORE_DB_S3_USE_SSL"),
				},
			},
		},
		auth: auth.Config{
			AllowedAnonymous:  viper.GetBool("ALLOWED_ANONYMOUS"),
			ManagementApiKeys: viper.GetBool("ALLOW_MANAGED_API_KEYS"),
			AnonymousRole:     viper.GetString("ANONYMOUS_ROLE"),
			ConfigFile:        viper.GetString("AUTH_CONFIG_FILE"),
		},
	}
}

func (c *Config) parseAuth() error {
	if c.auth.ConfigFile != "" {
		pc, err := auth.LoadFile(c.auth.ConfigFile)
		if err != nil {
			return err
		}
		c.Cluster.Node.Auth = *pc
	}
	// config for OIDC
	if c.OIDC.Issuer != "" {
		c.Cluster.Node.Auth.OIDC = c.OIDC
	}

	c.Cluster.Node.Auth.Anonymous.Allowed = c.auth.AllowedAnonymous
	c.Cluster.Node.Auth.Anonymous.Role = c.auth.AnonymousRole

	return nil
}
