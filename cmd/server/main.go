package main

import (
	"context"
	"database/sql"
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
	conf := loadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	isClusterMode := conf.Cluster.ManagementUrl != ""
	var hc hugr.Config
	if !isClusterMode {
		ac, err := conf.Auth.Configure()
		if err != nil {
			log.Println("Auth configuration error:", err)
			os.Exit(1)
		}
		hc = hugr.Config{
			AdminUI:            conf.EnableAdminUI,
			AdminUIFetchPath:   conf.AdminUIFetchPath,
			Debug:              conf.DebugMode,
			AllowParallel:      conf.AllowParallel,
			MaxParallelQueries: conf.MaxParallelQueries,
			MaxDepth:           conf.MaxDepthInTypes,
			DB:                 conf.DB,
			CoreDB:             coredb.New(conf.CoreDB),
			Auth:               ac,
			Cache:              conf.Cache,
		}
	}
	if isClusterMode {
		var err error
		hc, err = RegisterNode(ctx, conf.Cluster, conf)
		if err != nil {
			log.Println("Cluster registration error:", err)
			os.Exit(1)
		}
		defer UnregisterNode(ctx, conf.Cluster)
	}
	// Start the server

	engine := hugr.New(hc)

	if conf.DB.Path != "" {
		log.Println("DB path: ", conf.DB.Path)
	} else {
		log.Println("DB path is not set, using in-memory database")
	}

	if conf.CoreDB.Path != "" {
		log.Println("Core DB path: ", conf.CoreDB.Path)
	}

	if conf.CoreDB.Path == "" && conf.CoreDB.ReadOnly {
		log.Println("Core DB path is not set, using in-memory database, it can't be read-only")
		os.Exit(1)
	}

	if conf.CoreDB.Path == "" {
		log.Println("Core DB path is not set, using in-memory database")
	}
	if hc.Auth != nil {
		auth.PrintSummary(hc.Auth)
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
			ManagementNode: conf.Cluster.ManagementUrl,
			NodeName:       conf.Cluster.NodeName,
			NodeUrl:        conf.Cluster.NodeUrl,
			Secret:         conf.Cluster.Secret,
			Timeout:        conf.Cluster.Timeout,
		}))
		if err != nil {
			log.Println("Attach cluster source error:", err)
			os.Exit(1)
		}
	}

	srv := &http.Server{
		Addr:    conf.Bind,
		Handler: cors.Middleware(conf.Cors)(engine),
	}

	go func() {
		log.Println("Starting server on ", conf.Bind)
		if conf.DebugMode {
			log.Println("Debug mode on")
		}
		err := srv.ListenAndServe()
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
