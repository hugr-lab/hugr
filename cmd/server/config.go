package main

import (
	"time"

	"github.com/hugr-lab/hugr/pkg/auth"
	"github.com/hugr-lab/hugr/pkg/cors"
	"github.com/hugr-lab/query-engine/pkg/cache"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
	"github.com/hugr-lab/query-engine/pkg/db"
	"github.com/hugr-lab/query-engine/pkg/types"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Bind        string
	ServiceBind string
	Cluster     ClusterConfig

	EnableAdminUI      bool
	AdminUIFetchPath   string
	DebugMode          bool
	AllowParallel      bool
	MaxParallelQueries int

	MaxDepthInTypes int

	DB db.Config

	CoreDB coredb.Config

	Cors cors.Config
	Auth auth.Config

	Cache cache.Config
}

func init() {
	initEnvs()
}

func initEnvs() {
	_ = godotenv.Overload()
	viper.SetDefault("BIND", ":15000")
	viper.SetDefault("ADMIN_UI", true)
	viper.SetDefault("ADMIN_UI_FETCH_PATH", "")
	viper.SetDefault("DEBUG", false)
	viper.SetDefault("ALLOW_PARALLEL", true)
	viper.SetDefault("MAX_PARALLEL_QUERIES", 0)
	viper.SetDefault("MAX_DEPTH", 0)
	viper.SetDefault("DB_PATH", "")
	viper.SetDefault("DB_MAX_OPEN_CONNS", 0)
	viper.SetDefault("DB_MAX_IDLE_CONNS", 0)
	viper.SetDefault("ALLOWED_ANONYMOUS", true)
	viper.SetDefault("ANONYMOUS_ROLE", "admin")
	viper.SetDefault("CLUSTER_TIMEOUT", 5*time.Second)
	viper.AutomaticEnv()
}

func loadConfig() Config {
	return Config{
		Bind:        viper.GetString("BIND"),
		ServiceBind: viper.GetString("SERVICE_BIND"),
		Cluster: ClusterConfig{
			Secret:        viper.GetString("CLUSTER_SECRET"),
			ManagementUrl: viper.GetString("CLUSTER_MANAGEMENT_URL"),
			NodeName:      viper.GetString("CLUSTER_NODE_NAME"),
			NodeUrl:       viper.GetString("CLUSTER_NODE_URL"),
			Timeout:       viper.GetDuration("CLUSTER_TIMEOUT"),
		},
		EnableAdminUI:      viper.GetBool("ADMIN_UI"),
		AdminUIFetchPath:   viper.GetString("ADMIN_UI_FETCH_PATH"),
		DebugMode:          viper.GetBool("DEBUG"),
		AllowParallel:      viper.GetBool("ALLOW_PARALLEL"),
		MaxParallelQueries: viper.GetInt("MAX_PARALLEL_QUERIES"),
		MaxDepthInTypes:    viper.GetInt("MAX_DEPTH"),
		DB: db.Config{
			Path:         viper.GetString("DB_PATH"),
			MaxOpenConns: viper.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns: viper.GetInt("DB_MAX_IDLE_CONNS"),
			Settings: db.Settings{
				HomeDirectory:        viper.GetString("DB_HOME_DIRECTORY"),
				AllowedDirectories:   viper.GetStringSlice("DB_ALLOWED_DIRECTORIES"),
				AllowedPaths:         viper.GetStringSlice("DB_ALLOWED_PATHS"),
				EnableLogging:        viper.GetBool("DB_ENABLE_LOGGING"),
				MaxMemory:            viper.GetInt("DB_MAX_MEMORY"),
				MaxTempDirectorySize: viper.GetInt("DB_MAX_TEMP_DIRECTORY_SIZE"),
				TempDirectory:        viper.GetString("DB_TEMP_DIRECTORY"),
				WorkerThreads:        viper.GetInt("DB_WORKER_THREADS"),
				PGConnectionLimit:    viper.GetInt("DB_PG_CONNECTION_LIMIT"),
				PGPagesPerTask:       viper.GetInt("DB_PG_PAGES_PER_TASK"),
			},
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
		Cors: cors.Config{
			CorsAllowedOrigins: viper.GetStringSlice("CORS_ALLOWED_ORIGINS"),
			CorsAllowedHeaders: viper.GetStringSlice("CORS_ALLOWED_HEADERS"),
			CorsAllowedMethods: viper.GetStringSlice("CORS_ALLOWED_METHODS"),
		},
		Auth: auth.Config{
			AllowedAnonymous:  viper.GetBool("ALLOWED_ANONYMOUS"),
			ManagementApiKeys: viper.GetBool("ALLOW_MANAGED_API_KEYS"),
			AnonymousRole:     viper.GetString("ANONYMOUS_ROLE"),
			SecretKey:         viper.GetString("SECRET_KEY"),
			ConfigFile:        viper.GetString("AUTH_CONFIG_FILE"),
		},
		Cache: cache.Config{
			TTL: types.Interval(viper.GetDuration("CACHE_TTL")),
			L1: cache.L1Config{
				Enabled:      viper.GetBool("CACHE_L1_ENABLED"),
				MaxSize:      viper.GetInt("CACHE_L1_MAX_SIZE"),
				MaxItemSize:  viper.GetInt("CACHE_L1_MAX_ITEM_SIZE"),
				Shards:       viper.GetInt("CACHE_L1_SHARDS"),
				CleanTime:    types.Interval(viper.GetDuration("CACHE_L1_CLEAN_TIME")),
				EvictionTime: types.Interval(viper.GetDuration("CACHE_L1_EVICTION_TIME")),
			},
			L2: cache.L2Config{
				Enabled:   viper.GetBool("CACHE_L2_ENABLED"),
				Backend:   cache.BackendType(viper.GetString("CACHE_L2_BACKEND")),
				Addresses: viper.GetStringSlice("CACHE_L2_ADDRESSES"),
				Database:  viper.GetInt("CACHE_L2_DATABASE"),
				Username:  viper.GetString("CACHE_L2_USERNAME"),
				Password:  viper.GetString("CACHE_L2_PASSWORD"),
			},
		},
	}
}
