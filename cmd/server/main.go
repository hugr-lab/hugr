package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/hugr-lab/hugr/pkg/auth"
	"github.com/hugr-lab/hugr/pkg/auth/oauth"
	"github.com/hugr-lab/hugr/pkg/cors"
	"github.com/hugr-lab/hugr/pkg/info"
	"github.com/hugr-lab/hugr/pkg/service"
	hugr "github.com/hugr-lab/query-engine"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
)

var (
	installFlag = flag.Bool("install", false, "install duckdb dependencies")
)

func main() {
	flag.Parse()
	if *installFlag {
		err := installDuckDBExtension()
		if err != nil {
			log.Panicln(err)
		}
		return
	}
	config := loadConfig()

	// Validate TLS configuration
	var tlsCfg *tls.Config
	tlsEnabled := config.TLSCertFile != "" || config.TLSKeyFile != ""
	if tlsEnabled {
		if config.TLSCertFile == "" || config.TLSKeyFile == "" {
			log.Println("Both TLS_CERT_FILE and TLS_KEY_FILE must be set when enabling TLS")
			os.Exit(1)
		}
		cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			log.Printf("TLS configuration error: %v\n", err)
			os.Exit(1)
		}
		tlsCfg = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Validate cluster configuration
	if config.Cluster.Enabled {
		if config.Cluster.Role != "management" && config.Cluster.Role != "worker" {
			log.Printf("Invalid CLUSTER_ROLE=%q: must be 'management' or 'worker'\n", config.Cluster.Role)
			os.Exit(1)
		}
		if !strings.HasPrefix(config.CoreDB.Path, "postgres") {
			log.Println("Cluster mode requires PostgreSQL as CoreDB (CORE_DB_PATH must be a postgres:// DSN)")
			os.Exit(1)
		}
		log.Printf("Cluster mode: role=%s, node=%s\n", config.Cluster.Role, config.Cluster.NodeName)
	}

	authConfig, err := config.Auth.Configure(ctx)
	if err != nil {
		log.Println("Auth configuration error:", err)
		os.Exit(1)
	}

	hugrConfig := hugr.Config{
		AdminUI:               config.EnableAdminUI,
		AdminUIFetchPath:      config.AdminUIFetchPath,
		Debug:                 config.DebugMode,
		Profiling:             config.HttpProfiling,
		AllowParallel:         config.AllowParallel,
		MaxParallelQueries:    config.MaxParallelQueries,
		MaxDepth:              config.MaxDepthInTypes,
		SchemaCacheMaxEntries: config.SchemaCacheMaxEntries,
		SchemaCacheTTL:        config.SchemaCacheTTL,
		MCPEnabled:            config.MCPEnabled,
		DB:                    config.DB,
		CoreDB:                coredb.New(config.CoreDB),
		Auth:                  authConfig,
		Cache:                 config.Cache,
		Embedder:              config.Embedder,
		Cluster:               config.Cluster,
	}

	if config.DB.Path != "" {
		log.Println("DB path: ", config.DB.Path)
	} else {
		log.Println("DB path is not set, using in-memory database")
	}

	if config.CoreDB.Path != "" {
		log.Println("Core DB path: ", config.CoreDB.Path)
	}

	if config.CoreDB.Path == "" && config.CoreDB.ReadOnly {
		log.Println("Core DB path is not set, using in-memory database, it can't be read-only")
		os.Exit(1)
	}

	if config.CoreDB.Path == "" {
		log.Println("Core DB path is not set, using in-memory database")
	}

	engine, err := hugr.New(hugrConfig)
	if err != nil {
		log.Println("Engine creation error:", err)
		os.Exit(1)
	}

	if hugrConfig.Auth != nil {
		auth.PrintSummary(hugrConfig.Auth)
	}

	err = engine.AttachRuntimeSource(ctx, info.New(info.NodeInfo{
		Version:   Version,
		BuildDate: BuildDate,
		InCluster: config.Cluster.Enabled,
		NodeRole:  config.Cluster.Role,
		NodeName:  config.Cluster.NodeName,
	}))
	if err != nil {
		log.Println("Attach version source error:", err)
		os.Exit(1)
	}

	err = engine.Init(ctx)
	if err != nil {
		log.Println("Initialization error:", err)
		os.Exit(1)
	}
	defer engine.Close()

	var handler http.Handler = engine

	// Add /auth/config and OAuth proxy endpoints if OIDC is configured
	if config.Auth.OIDCEnabled() {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /auth/config", config.Auth.AuthConfigHandler())

		// Mount OAuth proxy for MCP clients when MCP is enabled
		mcpClientID := config.MCPOAuthClientID
		mcpClientSecret := config.MCPOAuthClientSecret
		// Fall back to OIDC client credentials if MCP-specific ones are not set
		if mcpClientID == "" {
			mcpClientID = config.Auth.OIDC.ClientID
		}
		if mcpClientSecret == "" {
			mcpClientSecret = config.Auth.OIDC.ClientSecret
		}
		if config.MCPEnabled && mcpClientSecret != "" {
			oauthProxy, err := oauth.NewProxy(ctx, oauth.Config{
				Issuer:       config.Auth.OIDC.Issuer,
				ClientID:     mcpClientID,
				ClientSecret: mcpClientSecret,
				Scopes:       config.Auth.OIDC.Scopes,
				RedirectURL:  config.Auth.OIDC.RedirectURL,
				TLSInsecure:  config.Auth.OIDC.TLSInsecure,
				SecretKey:    config.Auth.SecretKey,
			})
			if err != nil {
				log.Println("OAuth proxy initialization error:", err)
				os.Exit(1)
			}
			oauthProxy.RegisterHandlers(mux)
			log.Println("MCP OAuth proxy enabled")
		}

		mux.Handle("/", engine)
		handler = mux
	}

	srv := &http.Server{
		Addr:      config.Bind,
		Handler:   cors.Middleware(config.Cors)(handler),
		TLSConfig: tlsCfg,
	}

	go func() {
		if tlsEnabled {
			log.Printf("Starting server on %s (HTTPS)\n", config.Bind)
		} else {
			log.Printf("Starting server on %s (HTTP)\n", config.Bind)
		}
		if config.DebugMode {
			log.Println("Debug mode on")
		}
		var err error
		if tlsEnabled {
			err = srv.ListenAndServeTLS("", "")
		} else {
			err = srv.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			log.Println("Server stopped")
			return
		}
		if err != nil {
			log.Println("Server error:", err)
			os.Exit(1)
		}
	}()
	svc := service.New(config.ServiceBind)
	err = svc.Start(ctx)
	if err != nil {
		log.Println("Services endpoint server start error:", err)
	}
	<-ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	if err != nil {
		log.Println("Server shutdown error:", err)
		os.Exit(1)
	}
	err = svc.Stop(ctx)
	if err != nil {
		log.Println("Service shutdown error:", err)
		os.Exit(1)
	}
	log.Println("Server shutdown")
}

func installDuckDBExtension() error {
	connector, err := duckdb.NewConnector("", nil)
	if err != nil {
		return err
	}
	defer connector.Close()
	conn := sql.OpenDB(connector)
	defer conn.Close()

	_, err = conn.Exec(`
		INSTALL postgres; LOAD postgres;
		INSTALL spatial; LOAD spatial;
		INSTALL sqlite; LOAD sqlite;
		INSTALL sqlite3; LOAD sqlite3;
		INSTALL h3 FROM community; LOAD h3;
		--  INSTALL arrow; LOAD arrow;
		INSTALL aws; LOAD aws;
		INSTALL delta; LOAD delta;
		INSTALL httpfs; LOAD httpfs;
		INSTALL fts; LOAD fts;
		INSTALL iceberg; LOAD iceberg;
		INSTALL json; LOAD json;
		INSTALL parquet; LOAD parquet;
		INSTALL mysql; LOAD mysql;
		INSTALL vss; LOAD vss;
		INSTALL azure; LOAD azure;
		INSTALL mssql FROM community; LOAD mssql;
		INSTALL airport FROM community; LOAD airport;
		INSTALL ducklake; LOAD ducklake;
	`)
	if err != nil {
		return err
	}

	var version string
	err = conn.QueryRow(`SELECT version();`).Scan(&version)
	if err != nil {
		return err
	}
	log.Println("DuckDB version: ", version)
	rows, err := conn.Query(`
		SELECT extension_name, description, installed, install_path
		FROM duckdb_extensions();
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	log.Println("Installed extensions:")
	for rows.Next() {
		var name, desc, path string
		var installed bool
		err = rows.Scan(&name, &desc, &installed, &path)
		if err != nil {
			return err
		}
		log.Printf("Extension: %s, %s, installed: %t, path: %s\n", name, desc, installed, path)
	}

	return nil
}
