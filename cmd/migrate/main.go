package main

import (
	"database/sql"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
	"github.com/hugr-lab/query-engine/pkg/db"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	fCoreDB  = flag.String("core-db", "", "core database path")
	fPath    = flag.String("path", "/migrations", "path to the migrations folder")
	fVersion = flag.String("to-version", "", "version to migrate to")
	fVerbose = flag.Bool("verbose", false, "enable verbose logging")
)

func main() {
	flag.Parse()

	// check migrations path
	if *fPath == "" {
		log.Println("migrations path is not set")
		os.Exit(1)
	}
	// open duckdb database
	dbType := db.SDBDuckDB
	if strings.HasPrefix(*fCoreDB, "postgres://") {
		dbType = db.SDBPostgres
	}

	// check if database exists
	exists, err := checkDBExists(dbType, *fCoreDB)
	if err != nil {
		log.Println("failed to check if database exists:", err)
		os.Exit(1)
	}
	if !exists {
		log.Println("core database does not exist, will create it")
		err = initDB(dbType, *fCoreDB)
		if err != nil {
			log.Println("failed to create core database:", err)
			os.Exit(1)
		}
		log.Println("core database created")
		os.Exit(0)
	}

	conn, err := openDB(dbType, *fCoreDB)
	if err != nil {
		log.Println("failed to open core_db:", err)
		os.Exit(1)
	}
	defer conn.Close()

	var version string

	err = conn.QueryRow("SELECT version FROM version LIMIT 1;").Scan(&version)
	if err != nil {
		log.Println("failed to get current version:", err)
		os.Exit(1)
	}

	if *fVerbose && dbType != "postgres" {
		rows, err := conn.Query("SELECT database_name, schema_name, table_name FROM duckdb_tables();")
		if err != nil {
			log.Println("failed to get tables:", err)
			os.Exit(1)
		}
		defer rows.Close()
		for rows.Next() {
			var dbName, schemaName, tableName string
			err = rows.Scan(&dbName, &schemaName, &tableName)
			if err != nil {
				log.Println("failed to scan tables:", err)
				os.Exit(1)
			}
			log.Println("table:", dbName, schemaName, tableName)
		}
		rows.Close()
	}

	type file struct {
		path    string
		version string
	}
	var files []file

	err = filepath.WalkDir(*fPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		parts := strings.Split(strings.TrimLeft(strings.TrimPrefix(path, *fPath), string(filepath.Separator)), string(filepath.Separator))
		if len(parts) < 1 {
			return nil
		}
		mv := strings.TrimRight(parts[0], ".sql")
		if version >= mv || *fVersion != "" && mv > *fVersion {
			return nil
		}
		files = append(files, file{
			path:    path,
			version: mv,
		})
		return nil
	})
	if err != nil {
		log.Println("failed to walk migrations folder:", err)
		os.Exit(1)
	}

	slices.SortFunc(files, func(a, b file) int {
		return strings.Compare(a.path, b.path)
	})

	for _, f := range files {
		if *fVerbose {
			log.Println("applying migration:", f)
		}
		b, err := os.ReadFile(f.path)
		if err != nil {
			log.Println("failed to read migration:", err)
			os.Exit(1)
		}
		parsedSQL, err := db.ParseSQLScriptTemplate(dbType, string(b))
		if err != nil {
			log.Println("failed to parse migration:", err)
			os.Exit(1)
		}
		_, err = conn.Exec(parsedSQL)
		if err != nil {
			log.Println("failed to apply migration:", err)
			os.Exit(1)
		}
		if version != f.version {
			_, err = conn.Exec("UPDATE version SET version = $1;", version)
			if err != nil {
				log.Println("failed to update version:", err)
				os.Exit(1)
			}
			version = f.version
		}
	}
	_, err = conn.Exec("UPDATE version SET version = $1;", version)
	if err != nil {
		log.Println("failed to update version:", err)
		os.Exit(1)
	}

	log.Println("migrations applied successfully")
	if !strings.HasPrefix(*fCoreDB, "postgres://") {
		// shrink log file
		_, err = conn.Exec("PRAGMA enable_checkpoint_on_shutdown; PRAGMA force_checkpoint;")
		if err != nil {
			log.Println("failed to shrink log file:", err)
			os.Exit(1)
		}
	}
}

var errIsReadOnly = errors.New("database is read-only")

func checkDBExists(dbType db.ScriptDBType, dbPath string) (bool, error) {
	switch dbType {
	case "postgres":
		db, err := sql.Open("pgx", dbPath)
		if err != nil {
			return false, err
		}
		defer db.Close()
		err = db.Ping()
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "3D000" {
				return false, nil
			}
			return false, err
		}
		if err != nil {
			return false, err
		}
	case "duckdb":
		if strings.HasPrefix(dbPath, "s3://") {
			return false, errIsReadOnly
		}
		// check file exists
		_, err := os.Stat(dbPath)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		db, err := sql.Open("duckdb", dbPath)
		if err != nil {
			return false, err
		}
		defer db.Close()
		err = db.Ping()
		if err != nil {
			return false, err
		}
	default:
		return false, nil
	}
	return true, nil
}

func openDB(dbType db.ScriptDBType, dbPath string) (*sql.DB, error) {
	switch dbType {
	case db.SDBPostgres:
		db, err := sql.Open("pgx", dbPath)
		if err != nil {
			return nil, err
		}
		return db, nil
	case db.SDBDuckDB:
		if strings.HasPrefix(dbPath, "s3://") {
			return nil, errIsReadOnly
		}
		db, err := sql.Open("duckdb", dbPath)
		if err != nil {
			return nil, err
		}
		return db, nil
	default:
		return nil, errors.New("unsupported database type")
	}
}

func initDB(dbType db.ScriptDBType, dbPath string) error {
	var d *sql.DB
	var err error
	switch dbType {
	case db.SDBPostgres:
		// try to create the database (need to connect to the postgres database)
		dbDSN, err := sources.ParseDSN(dbPath)
		if err != nil {
			return err
		}
		dbName := dbDSN.DBName
		dbDSN.DBName = "postgres"
		d, err = sql.Open("pgx", dbDSN.String())
		if err != nil {
			return err
		}
		_, err = d.Exec("CREATE DATABASE \"" + dbName + "\";")
		d.Close()
		if err != nil {
			return err
		}
		d, err = sql.Open("pgx", dbPath)
	case db.SDBDuckDB:
		if strings.HasPrefix(dbPath, "s3://") {
			return errIsReadOnly
		}
		d, err = sql.Open("duckdb", dbPath)
	default:
		return errors.New("unsupported database type")
	}
	if err != nil {
		return err
	}
	defer d.Close()
	sql, err := db.ParseSQLScriptTemplate(dbType, coredb.InitSchema)
	if err != nil {
		return err
	}
	_, err = d.Exec(sql)
	return err
}
