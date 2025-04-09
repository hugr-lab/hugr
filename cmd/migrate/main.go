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

	"github.com/marcboeker/go-duckdb/v2"
)

var (
	fCoreDB  = flag.String("core-db", "../../.local/qe-core.duckdb", "core database path")
	fPath    = flag.String("path", "migrations", "path to the migrations folder")
	fVersion = flag.String("to-version", "", "version to migrate to")
)

func main() {
	flag.Parse()

	// check migrations path
	if *fPath == "" {
		log.Println("migrations path is not set")
		os.Exit(1)
	}

	ff, err := os.ReadDir(*fPath)
	if err != nil {
		log.Println("failed to read migrations folder:", err)
		os.Exit(1)
	}

	slices.SortFunc(ff, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})

	// open duckdb database
	c, err := duckdb.NewConnector(*fCoreDB, nil)
	if err != nil {
		log.Println("failed to open core db:", err)
		os.Exit(1)
	}
	defer c.Close()

	db := sql.OpenDB(c)
	defer db.Close()

	var version string

	err = db.QueryRow("SELECT version FROM version LIMIT 1;").Scan(&version)
	var de *duckdb.Error
	if err != nil && (!errors.As(err, &de) || de.Type != duckdb.ErrorTypeCatalog) {
		log.Println("failed to get current version:", err)
		os.Exit(1)
	}

	rows, err := db.Query("SELECT database_name, schema_name, table_name FROM duckdb_tables();")
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
		parts := strings.SplitN(path, string(filepath.Separator), 3)
		if len(parts) < 2 {
			return nil
		}
		mv := strings.TrimRight(parts[1], ".sql")
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
		log.Println("applying migration:", f)
		b, err := os.ReadFile(f.path)
		if err != nil {
			log.Println("failed to read migration:", err)
			os.Exit(1)
		}
		_, err = db.Exec(string(b))
		if err != nil {
			log.Println("failed to apply migration:", err)
			os.Exit(1)
		}
		if version != f.version {
			_, err = db.Exec("UPDATE version SET version = $1;", version)
			if err != nil {
				log.Println("failed to update version:", err)
				os.Exit(1)
			}
			version = f.version
		}
	}
	_, err = db.Exec("UPDATE version SET version = $1;", version)
	if err != nil {
		log.Println("failed to update version:", err)
		os.Exit(1)
	}

	log.Println("migrations applied successfully")
	// shrink log file
	_, err = db.Exec("PRAGMA enable_checkpoint_on_shutdown; PRAGMA force_checkpoint;")
	if err != nil {
		log.Println("failed to shrink log file:", err)
		os.Exit(1)
	}
}
