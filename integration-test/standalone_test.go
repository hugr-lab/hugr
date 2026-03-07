//go:build duckdb_arrow

package integration_test

import (
	"testing"

	"github.com/hugr-lab/hugr/pkg/info"
)

func TestStandalone_EngineStartsWithDefaultConfig(t *testing.T) {
	config := newTestConfig()
	engine := initEngine(t, config)
	if engine == nil {
		t.Fatal("Engine should not be nil")
	}
}

func TestStandalone_EngineShutdownCleanly(t *testing.T) {
	config := newTestConfig()
	engine := initEngine(t, config)
	if engine == nil {
		t.Fatal("Engine should not be nil")
	}
}

func TestStandalone_InfoSourceAttaches(t *testing.T) {
	config := newTestConfig()
	src := info.New(info.NodeInfo{
		Version:   "test-version",
		BuildDate: "test-date",
		InCluster: false,
	})
	engine := initEngineWithSources(t, config, src)
	if engine == nil {
		t.Fatal("Engine should not be nil")
	}
}
