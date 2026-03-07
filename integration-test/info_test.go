//go:build duckdb_arrow

package integration_test

import (
	"testing"

	"github.com/hugr-lab/hugr/pkg/info"
)

func TestInfo_StandaloneClusterModeFalse(t *testing.T) {
	config := newTestConfig()
	src := info.New(info.NodeInfo{
		Version:   "1.0.0",
		BuildDate: "2026-03-07",
		InCluster: false,
	})
	engine := initEngineWithSources(t, config, src)
	if engine == nil {
		t.Fatal("Engine should not be nil")
	}
}

// Management and worker info tests require PostgreSQL CoreDB.
// See integration-test/e2e/ for full cluster e2e tests.
