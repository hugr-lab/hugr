package info

import (
	"context"
	"encoding/json"

	hugr "github.com/hugr-lab/query-engine"
	cs "github.com/hugr-lab/query-engine/pkg/catalogs/sources"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime"
	"github.com/hugr-lab/query-engine/pkg/db"
	"github.com/hugr-lab/query-engine/pkg/engines"
	"github.com/marcboeker/go-duckdb/v2"

	_ "embed"
)

// The Version runtime source for hugr query engine that expose version management methods:
// 1. core.version
// 2. core.info

//go:embed schema.graphql
var schema string

type NodeInfo struct {
	Version        string    `json:"version"`
	BuildDate      string    `json:"build_date"`
	InCluster      bool      `json:"cluster_mode"`
	ManagementNode string    `json:"management_node"`
	Engine         hugr.Info `json:"engine"`
}

func (v *NodeInfo) toDuckdb() (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var info map[string]any
	err = json.Unmarshal(b, &info)
	if err != nil {
		return nil, err
	}

	return info, nil
}

var _ (sources.RuntimeSource) = (*Source)(nil)

type Source struct {
	info NodeInfo
}

func New(info NodeInfo) *Source {
	return &Source{
		info: info,
	}
}

func (*Source) Name() string {
	return "core.version"
}

func (*Source) Engine() engines.Engine {
	return engines.NewDuckDB()
}

func (*Source) IsReadonly() bool {
	return true
}

func (*Source) AsModule() bool {
	return false
}

func (s *Source) Attach(ctx context.Context, pool *db.Pool) error {
	// Register the version UDF
	err := pool.RegisterScalarFunction(ctx, &db.ScalarFunctionNoArgs[NodeInfo]{
		Name: "node_version",
		Execute: func(ctx context.Context) (NodeInfo, error) {
			return s.info, nil
		},
		OutputType: runtime.DuckDBStructTypeFromSchemaMust(map[string]any{
			"version":    duckdb.TYPE_VARCHAR,
			"build_date": duckdb.TYPE_VARCHAR,
		}),
		ConvertOutput: func(out NodeInfo) (any, error) {
			return map[string]any{
				"version":    out.Version,
				"build_date": out.BuildDate,
			}, nil
		},
	})
	if err != nil {
		return err
	}

	// Register the node info UDF
	err = pool.RegisterScalarFunction(ctx, &db.ScalarFunctionNoArgs[NodeInfo]{
		Name: "node_info",
		Execute: func(ctx context.Context) (NodeInfo, error) {
			return s.info, nil
		},
		OutputType: duckdbNodeInfoType,
		ConvertOutput: func(out NodeInfo) (any, error) {
			return out.toDuckdb()
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (*Source) Catalog(ctx context.Context) cs.Source {
	return cs.NewStringSource("version/schema.graphql", schema)
}

var duckdbNodeInfoType = runtime.DuckDBStructTypeFromSchemaMust(map[string]any{
	"version":         duckdb.TYPE_VARCHAR,
	"build_date":      duckdb.TYPE_VARCHAR,
	"management_node": duckdb.TYPE_VARCHAR,
	"cluster_mode":    duckdb.TYPE_BOOLEAN,
	"engine": map[string]any{
		"admin_ui":             duckdb.TYPE_BOOLEAN,
		"debug":                duckdb.TYPE_BOOLEAN,
		"allow_parallel":       duckdb.TYPE_BOOLEAN,
		"max_parallel_queries": duckdb.TYPE_INTEGER,
		"max_depth":            duckdb.TYPE_INTEGER,
		"duckdb": map[string]any{
			"path":           duckdb.TYPE_VARCHAR,
			"max_open_conns": duckdb.TYPE_INTEGER,
			"max_idle_conns": duckdb.TYPE_INTEGER,
			"settings": map[string]any{
				"allowed_directories":     []duckdb.Type{duckdb.TYPE_VARCHAR},
				"allowed_paths":           []duckdb.Type{duckdb.TYPE_VARCHAR},
				"enable_logging":          duckdb.TYPE_BOOLEAN,
				"max_memory":              duckdb.TYPE_INTEGER,
				"max_temp_directory_size": duckdb.TYPE_INTEGER,
				"temp_directory":          duckdb.TYPE_VARCHAR,
				"worker_threads":          duckdb.TYPE_INTEGER,
				"pg_connection_limit":     duckdb.TYPE_INTEGER,
				"pg_pages_per_task":       duckdb.TYPE_INTEGER,
			},
		},
		"coredb": map[string]any{
			"version": duckdb.TYPE_VARCHAR,
			"type":    duckdb.TYPE_VARCHAR,
		},
		"auth": []map[string]any{{
			"name": duckdb.TYPE_VARCHAR,
			"type": duckdb.TYPE_VARCHAR,
		}},
		"cache": map[string]any{
			"ttl": duckdb.TYPE_VARCHAR,
			"l1": map[string]any{
				"enabled":       duckdb.TYPE_BOOLEAN,
				"max_size":      duckdb.TYPE_INTEGER,
				"max_item_size": duckdb.TYPE_INTEGER,
				"shards":        duckdb.TYPE_INTEGER,
				"clean_time":    duckdb.TYPE_VARCHAR,
				"eviction_time": duckdb.TYPE_VARCHAR,
			},
			"l2": map[string]any{
				"enabled":   duckdb.TYPE_BOOLEAN,
				"backend":   duckdb.TYPE_VARCHAR,
				"addresses": []duckdb.Type{duckdb.TYPE_VARCHAR},
				"database":  duckdb.TYPE_INTEGER,
				"password":  duckdb.TYPE_VARCHAR,
				"username":  duckdb.TYPE_VARCHAR,
			},
		},
	},
})
