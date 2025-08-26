package cluster

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	cs "github.com/hugr-lab/query-engine/pkg/catalogs/sources"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/storage"
	"github.com/hugr-lab/query-engine/pkg/db"
	"github.com/hugr-lab/query-engine/pkg/engines"
	"github.com/hugr-lab/query-engine/pkg/types"
	"github.com/marcboeker/go-duckdb/v2"

	_ "embed"
)

// The runtime source for hugr query engine that expose cluster management methods:
// 1. cluster.load_data_source
// 2. cluster.unload_data_source
// 3. cluster.register_s3
// 4. cluster.unregister_s3
// 5. cluster.registered_s3
// 6. cluster.version
// 7. cluster.nodes

//go:embed schema.graphql
var schema string

var _ (sources.RuntimeSource) = (*Source)(nil)

type SourceConfig struct {
	NodeName       string        `json:"node_name"`
	NodeUrl        string        `json:"node_url"`
	Secret         string        `json:"secret"`
	Timeout        time.Duration `json:"timeout"`
	ManagementNode string        `json:"management_node"`
}

type Source struct {
	config SourceConfig
	c      *http.Client
}

func NewSource(config SourceConfig) *Source {
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	c := &http.Client{
		Timeout: config.Timeout,
		Transport: &clusterTransport{
			secret: config.Secret,
		},
	}
	return &Source{
		config: config,
		c:      c,
	}
}

type clusterTransport struct {
	secret string
}

func (t *clusterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("x-hugr-secret", t.secret)
	return http.DefaultTransport.RoundTrip(req)
}

func (*Source) Name() string {
	return "core.cluster"
}

func (*Source) Engine() engines.Engine {
	return engines.NewDuckDB()
}

func (*Source) IsReadonly() bool {
	return false
}

func (*Source) AsModule() bool {
	return true
}

func (s *Source) Attach(ctx context.Context, pool *db.Pool) error {
	// register views
	duckdb.RegisterReplacementScan(pool.Connector(), func(tableName string) (string, []any, error) {
		switch tableName {
		case "core_cluster_registered_storages":
			return "core_cluster_registered_storages", nil, nil
		case "core_cluster_nodes":
			return "core_cluster_nodes", nil, nil
		default:
			return "", nil, &duckdb.Error{
				Type: duckdb.ErrorTypeCatalog,
			}
		}
	})

	err := pool.RegisterTableRowFunction(ctx,
		&db.TableRowFunctionWithArgs[string, DataSourceStatus]{
			Name: "core_cluster_data_source_status",
			Arguments: []duckdb.TypeInfo{
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
			},
			ConvertArgs: func(named map[string]any, args ...any) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("invalid number of arguments")
				}
				name, ok := args[0].(string)
				if !ok {
					return "", fmt.Errorf("invalid argument type")
				}
				return name, nil
			},
			Execute: func(ctx context.Context, name string) ([]DataSourceStatus, error) {
				return s.DataSourceStatus(ctx, name)
			},
			ColumnInfos: []duckdb.ColumnInfo{
				{Name: "node", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "status", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "error", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
			},
			FillRow: func(out DataSourceStatus, row duckdb.Row) error {
				err := duckdb.SetRowValue(row, 0, out.Node)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 1, out.Status)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 2, out.Error)
				if err != nil {
					return err
				}
				return nil
			},
		},
	)
	if err != nil {
		return err
	}

	err = pool.RegisterTableRowFunction(ctx,
		&db.TableRowFunctionNoArgs[NodeStatus]{
			Name: "core_cluster_nodes",
			Execute: func(ctx context.Context) ([]NodeStatus, error) {
				return s.Nodes(ctx)
			},
			ColumnInfos: []duckdb.ColumnInfo{
				{Name: "name", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "version", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "error", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "ready", T: runtime.DuckDBTypeInfoByNameMust("BOOLEAN")},
				{Name: "last_seen", T: runtime.DuckDBTypeInfoByNameMust("TIMESTAMP")},
				{Name: "url", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
			},
			FillRow: func(out NodeStatus, row duckdb.Row) error {
				err := duckdb.SetRowValue(row, 0, out.Name)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 1, out.Version)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 2, out.Error)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 3, out.IsReady)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 4, out.LastSeen)
				if err != nil {
					return err
				}
				return duckdb.SetRowValue(row, 5, out.Url)
			},
		},
	)
	if err != nil {
		return err
	}

	err = pool.RegisterTableRowFunction(ctx,
		&db.TableRowFunctionNoArgs[StorageInfo]{
			Name: "core_cluster_registered_object_storages",
			Execute: func(ctx context.Context) ([]StorageInfo, error) {
				return s.RegisteredObjectStorages(ctx)
			},
			ColumnInfos: []duckdb.ColumnInfo{
				{Name: "node", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "name", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "type", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
				{Name: "scope", T: runtime.DuckDBListInfoByNameMust("VARCHAR")},
				{Name: "parameters", T: runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
			},
			FillRow: func(out StorageInfo, row duckdb.Row) error {
				err := duckdb.SetRowValue(row, 0, out.Node)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 1, out.Name)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 2, out.Type)
				if err != nil {
					return err
				}
				err = duckdb.SetRowValue(row, 3, out.Parameters)
				if err != nil {
					return err
				}
				return nil
			},
		},
	)
	if err != nil {
		return err
	}

	// register version
	err = pool.RegisterScalarFunction(ctx,
		&db.ScalarFunctionNoArgs[string]{
			Name: "core_cluster_version",
			Execute: func(ctx context.Context) (string, error) {
				return s.ClusterVersion(ctx), nil
			},
			OutputType: runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
			ConvertOutput: func(out string) (any, error) {
				return out, nil
			},
		})
	if err != nil {
		return err
	}

	// register object storage
	err = pool.RegisterScalarFunction(ctx,
		&db.ScalarFunctionWithArgs[storage.SecretInfo, *types.OperationResult]{
			Name: "core_cluster_register_object_storage",
			Execute: func(ctx context.Context, info storage.SecretInfo) (*types.OperationResult, error) {
				err := s.RegisterObjectStorage(ctx, info)
				if err != nil {
					return types.ErrResult(err), nil
				}
				return types.Result("Object storage registered", 1, 0), nil
			},
			ConvertInput: func(args []driver.Value) (storage.SecretInfo, error) {
				if len(args) != 12 {
					return storage.SecretInfo{}, fmt.Errorf("invalid number of arguments")
				}
				params := make(map[string]any)
				for i := 3; i < 12; i++ {
					if args[i] == nil {
						continue
					}
					switch i {
					case 3:
						params["KEY_ID"] = args[i].(string)
					case 4:
						params["SECRET"] = args[i].(string)
					case 5:
						if args[i].(string) != "" {
							params["REGION"] = args[i].(string)
						}
					case 6:
						params["ENDPOINT"] = args[i].(string)
					case 7:
						params["USE_SSL"] = args[i].(bool)
					case 8:
						params["URL_STYLE"] = args[i].(string)
					case 9:
						if args[i].(bool) {
							params["URL_COMPATIBILITY_MODE"] = true
						}
					case 10:
						if args[i].(string) != "" {
							params["KMS_KEY_ID"] = args[i].(string)
						}
					case 11:
						if args[i].(string) != "" {
							params["ACCOUNT_ID"] = args[i].(string)
						}
					}
				}
				return storage.SecretInfo{
					Type:       args[0].(string),
					Name:       args[1].(string),
					Scope:      args[2].(string),
					Parameters: params,
				}, nil
			},
			ConvertOutput: func(out *types.OperationResult) (any, error) {
				return out.ToDuckdb(), nil
			},
			InputTypes: []duckdb.TypeInfo{
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("BOOLEAN"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("BOOLEAN"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
				runtime.DuckDBTypeInfoByNameMust("VARCHAR"),
			},
			OutputType: types.DuckDBOperationResult(),
		})
	if err != nil {
		return err
	}

	err = pool.RegisterScalarFunction(ctx,
		&db.ScalarFunctionWithArgs[string, *types.OperationResult]{
			Name: "core_cluster_unregister_object_storage",
			Execute: func(ctx context.Context, name string) (*types.OperationResult, error) {
				err := s.UnregisterObjectStorage(ctx, name)
				if err != nil {
					return types.ErrResult(err), nil
				}
				return types.Result("S3 storage unregistered", 1, 0), nil
			},
			ConvertInput: func(args []driver.Value) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("invalid number of arguments")
				}
				name := args[0].(string)
				return name, nil
			},
			ConvertOutput: func(out *types.OperationResult) (any, error) {
				return out.ToDuckdb(), nil
			},
			InputTypes: []duckdb.TypeInfo{runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
			OutputType: types.DuckDBOperationResult(),
		})
	if err != nil {
		return err
	}

	err = pool.RegisterScalarFunction(ctx, &db.ScalarFunctionWithArgs[string, *types.OperationResult]{
		Name: "core_cluster_load_data_source",
		Execute: func(ctx context.Context, name string) (*types.OperationResult, error) {
			err := s.LoadDataSource(ctx, name)
			if err != nil {
				return types.ErrResult(err), nil
			}
			return types.Result("Datasource was loaded", 0, 0), nil
		},
		ConvertInput: func(args []driver.Value) (string, error) {
			if len(args) != 1 {
				return "", errors.New("invalid number of arguments")
			}
			name := args[0].(string)
			return name, nil
		},
		ConvertOutput: func(out *types.OperationResult) (any, error) {
			return out.ToDuckdb(), nil
		},
		InputTypes: []duckdb.TypeInfo{runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
		OutputType: types.DuckDBOperationResult(),
	})
	if err != nil {
		return err
	}

	err = pool.RegisterScalarFunction(ctx, &db.ScalarFunctionWithArgs[string, *types.OperationResult]{
		Name: "core_cluster_unload_data_source",
		Execute: func(ctx context.Context, name string) (*types.OperationResult, error) {
			err := s.UnloadDataSource(ctx, name)
			if err != nil {
				return types.ErrResult(err), nil
			}
			return types.Result("Datasource was unloaded", 0, 0), nil
		},
		ConvertInput: func(args []driver.Value) (string, error) {
			if len(args) != 1 {
				return "", errors.New("invalid number of arguments")
			}
			name := args[0].(string)
			return name, nil
		},
		ConvertOutput: func(out *types.OperationResult) (any, error) {
			return out.ToDuckdb(), nil
		},
		InputTypes: []duckdb.TypeInfo{runtime.DuckDBTypeInfoByNameMust("VARCHAR")},
		OutputType: types.DuckDBOperationResult(),
	})
	if err != nil {
		return err
	}

	return nil
}

func (*Source) Catalog(ctx context.Context) cs.Source {
	return cs.NewStringSource("core.cluster", schema)
}

// cluster management methods

func (s *Source) ClusterVersion(ctx context.Context) string {
	res, err := s.c.Get(s.config.ManagementNode + "/version")
	if err != nil {
		return ""
	}
	if res.StatusCode != http.StatusOK {
		return ""
	}
	var version string
	err = json.NewDecoder(res.Body).Decode(&version)
	if err != nil {
		return ""
	}
	return version
}

func (s *Source) Nodes(ctx context.Context) ([]NodeStatus, error) {
	res, err := s.c.Get(s.config.ManagementNode + "/node")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err != nil && len(b) > 0 {
			errMsg = string(b)
		}
		return nil, fmt.Errorf("load nodes: %s", errMsg)
	}
	var ns []NodeStatus
	err = json.NewDecoder(res.Body).Decode(&ns)
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (s *Source) LoadDataSource(ctx context.Context, name string) error {
	res, err := s.c.Get(s.config.ManagementNode + "/data-source/" + name + "/load")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err == nil && len(b) > 0 {
			errMsg = string(b)
		}
		return fmt.Errorf("load data source: %s", errMsg)
	}
	return nil
}

func (s *Source) UnloadDataSource(ctx context.Context, name string) error {
	res, err := s.c.Get(s.config.ManagementNode + "/data-source/" + name + "/unload")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err == nil && len(b) > 0 {
			errMsg = string(b)
		}
		return fmt.Errorf("load data source: %s", errMsg)
	}
	return nil
}

func (s *Source) DataSourceStatus(ctx context.Context, name string) ([]DataSourceStatus, error) {
	res, err := s.c.Get(s.config.ManagementNode + "/data-source/" + name + "/status")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err == nil && len(b) > 0 {
			errMsg = string(b)
		}
		return nil, fmt.Errorf("retrieve data source: %s", errMsg)
	}
	var ss []DataSourceStatus
	err = json.NewDecoder(res.Body).Decode(&ss)
	if err != nil {
		return nil, err
	}
	return ss, nil
}

func (s *Source) RegisterObjectStorage(ctx context.Context, info storage.SecretInfo) error {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(info)
	if err != nil {
		return err
	}
	res, err := s.c.Post(s.config.ManagementNode+"/storages", "application/json", &buf)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err == nil && len(b) > 0 {
			errMsg = string(b)
		}
		return fmt.Errorf("register storage: %s", errMsg)
	}
	return nil
}

func (s *Source) UnregisterObjectStorage(ctx context.Context, name string) error {
	req, err := http.NewRequest(http.MethodDelete, s.config.ManagementNode+"/storages/"+name, nil)
	if err != nil {
		return err
	}
	res, err := s.c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err == nil && len(b) > 0 {
			errMsg = string(b)
		}
		return fmt.Errorf("unregister object storage: %s", errMsg)
	}
	return nil
}

func (s *Source) RegisteredObjectStorages(ctx context.Context) ([]StorageInfo, error) {
	res, err := s.c.Get(s.config.ManagementNode + "/storages")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := res.Status
		if b, err := io.ReadAll(res.Body); err == nil && len(b) > 0 {
			errMsg = string(b)
		}
		return nil, fmt.Errorf("registered s3: %s", errMsg)
	}
	var ss []StorageInfo
	err = json.NewDecoder(res.Body).Decode(&ss)
	if err != nil {
		return nil, err
	}
	return ss, nil
}
