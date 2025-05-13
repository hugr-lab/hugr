package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/hugr-lab/hugr/pkg/auth"
	"github.com/hugr-lab/hugr/pkg/cluster"
	"github.com/hugr-lab/hugr/pkg/cors"
	"github.com/hugr-lab/hugr/pkg/info"
	hugr "github.com/hugr-lab/query-engine"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
	"github.com/marcboeker/go-duckdb/v2"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	isClusterMode := config.Cluster.ManagementUrl != ""
	var hugrConfig hugr.Config
	if !isClusterMode {
		authConfig, err := config.Auth.Configure()
		if err != nil {
			log.Println("Auth configuration error:", err)
			os.Exit(1)
		}
		hugrConfig = hugr.Config{
			AdminUI:            config.EnableAdminUI,
			AdminUIFetchPath:   config.AdminUIFetchPath,
			Debug:              config.DebugMode,
			AllowParallel:      config.AllowParallel,
			MaxParallelQueries: config.MaxParallelQueries,
			MaxDepth:           config.MaxDepthInTypes,
			DB:                 config.DB,
			CoreDB:             coredb.New(config.CoreDB),
			Auth:               authConfig,
			Cache:              config.Cache,
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
	}
	if isClusterMode {
		var err error
		hugrConfig, err = RegisterNode(ctx, config.Cluster, config)
		if err != nil {
			log.Println("Cluster registration error:", err)
			os.Exit(1)
		}
		defer UnregisterNode(ctx, config.Cluster)

		log.Printf("Cluster node %s registered at %s\n", config.Cluster.NodeName, config.Cluster.ManagementUrl)

	}
	// Start the server

	engine := hugr.New(hugrConfig)

	if hugrConfig.Auth != nil {
		auth.PrintSummary(hugrConfig.Auth)
	}

	err := engine.Init(ctx)
	if err != nil {
		log.Println("Initialization error:", err)
		os.Exit(1)
	}
	defer engine.Close()

	err = engine.AttachRuntimeSource(ctx, info.New(info.NodeInfo{
		Version:   Version,
		BuildDate: BuildDate,
		Engine:    engine.Info(),
	}))
	if err != nil {
		log.Println("Attach version source error:", err)
		os.Exit(1)
	}
	if isClusterMode {
		err = engine.AttachRuntimeSource(ctx, cluster.NewSource(cluster.SourceConfig{
			ManagementNode: config.Cluster.ManagementUrl,
			NodeName:       config.Cluster.NodeName,
			NodeUrl:        config.Cluster.NodeUrl,
			Secret:         config.Cluster.Secret,
			Timeout:        config.Cluster.Timeout,
		}))
		if err != nil {
			log.Println("Attach cluster source error:", err)
			os.Exit(1)
		}
	}

	srv := &http.Server{
		Addr:    config.Bind,
		Handler: cors.Middleware(config.Cors)(engine),
	}

	go func() {
		log.Println("Starting server on ", config.Bind)
		if config.DebugMode {
			log.Println("Debug mode on")
		}
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			log.Println("Server stopped")
			return
		}
		if err != nil {
			log.Println("Server error:", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	if err != nil {
		log.Println("Server shutdown error:", err)
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
