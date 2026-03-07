//go:build duckdb_arrow

package integration_test

import (
	"context"
	"testing"

	hugr "github.com/hugr-lab/query-engine"
	"github.com/hugr-lab/query-engine/pkg/auth"
	"github.com/hugr-lab/query-engine/pkg/cluster"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
)

func newTestConfig() hugr.Config {
	return hugr.Config{
		CoreDB: coredb.New(coredb.Config{}),
		Auth:   &auth.Config{},
	}
}

func newClusterConfig(role string) cluster.ClusterConfig {
	return cluster.ClusterConfig{
		Enabled:  true,
		Role:     role,
		NodeName: role + "-test",
		NodeURL:  "http://localhost:15000/ipc",
		Secret:   "test-secret",
	}
}

func newEngineWithError(config hugr.Config) (*hugr.Service, error) {
	engine, err := hugr.New(config)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	err = engine.Init(ctx)
	if err != nil {
		return nil, err
	}
	return engine, nil
}

func initEngine(t *testing.T, config hugr.Config) *hugr.Service {
	t.Helper()
	return initEngineWithSources(t, config)
}

func initEngineWithSources(t *testing.T, config hugr.Config, rtSources ...sources.RuntimeSource) *hugr.Service {
	t.Helper()
	engine, err := hugr.New(config)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	ctx := context.Background()
	for _, src := range rtSources {
		err = engine.AttachRuntimeSource(ctx, src)
		if err != nil {
			t.Fatalf("Failed to attach runtime source %s: %v", src.Name(), err)
		}
	}
	err = engine.Init(ctx)
	if err != nil {
		t.Fatalf("Failed to init engine: %v", err)
	}
	t.Cleanup(func() {
		engine.Close()
	})
	return engine
}
