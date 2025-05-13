package cluster

import (
	"context"
	"errors"
	"sync"
	"time"

	hugr "github.com/hugr-lab/query-engine"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/storage"
	"github.com/hugr-lab/query-engine/pkg/types"
)

var errNotReady = errors.New("node is not ready")

type Node struct {
	url            string
	c              *hugr.Client
	clusterVersion string
	check          time.Duration
	timeout        time.Duration
	stop           context.CancelFunc

	mu       sync.Mutex
	lastSeen time.Time
	version  string
	err      error
}

type NodeConfig struct {
	URL            string        `json:"url"`
	Version        string        `json:"version"`
	Secret         string        `json:"secret"`
	Timeout        time.Duration `json:"timeout"`
	Interval       time.Duration `json:"interval"`
	ClusterVersion string        `json:"cluster_version"`
}

func NewNode(ctx context.Context, c NodeConfig) (*Node, error) {
	if c.Interval == 0 {
		c.Interval = 60 * time.Second
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	n := &Node{
		c: hugr.NewClient(c.URL,
			hugr.WithApiKeyCustomHeader(c.Secret, "x-hugr-secret"),
			hugr.WithTimeout(c.Timeout),
			hugr.WithUserInfo("cluster", "cluster"),
			hugr.WithUserRole("admin"),
		),
		check:          c.Interval,
		timeout:        c.Timeout,
		clusterVersion: c.ClusterVersion,
		version:        c.Version,
		lastSeen:       time.Now(),
		url:            c.URL,
	}
	return n, nil
}

func (n *Node) Start(ctx context.Context) {
	if n.stop != nil {
		n.stop()
		time.Sleep(n.timeout)
	}
	ctx, n.stop = context.WithCancel(ctx)
	t := time.NewTicker(n.check)
	defer t.Stop()
	defer n.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			nv, err := n.c.Ping(ctx)
			n.mu.Lock()
			if err != nil {
				n.err = err
			} else {
				n.version = nv
				n.lastSeen = time.Now()
				n.err = nil
			}
			n.mu.Unlock()

		}
	}
}

func (n *Node) Stop() {
	if n.stop != nil {
		n.stop()
		n.stop = nil
	}
}

func (n *Node) IsReady() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.err == nil && n.version == n.clusterVersion
}

func (n *Node) Version() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.version
}

func (n *Node) LastSeen() time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastSeen
}

func (n *Node) Error() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return n.err.Error()
	}
	return ""
}

func (n *Node) LoadDataSource(ctx context.Context, name string) error {
	if !n.IsReady() {
		return errors.New("node is not ready")
	}
	return n.c.LoadDataSource(ctx, name)
}

func (n *Node) UnloadDataSource(ctx context.Context, name string) error {
	if !n.IsReady() {
		return errors.New("node is not ready")
	}
	return n.c.UnloadDataSource(ctx, name)
}

func (n *Node) DataSourceStatus(ctx context.Context, name string) (string, error) {
	if !n.IsReady() {
		return "", errors.New("node is not ready")
	}
	return n.c.DataSourceStatus(ctx, name)
}

func (n *Node) RegisterS3(ctx context.Context, info storage.S3Info) error {
	if !n.IsReady() {
		return errors.New("node is not ready")
	}
	res, err := n.c.Query(ctx, `mutation(
		$name: String!,
		$key: String!,
		$secret: String!,
		$region: String!,
		$endpoint: String!,
		$scope: String!,
		$use_ssl: Boolean!,
		$url_style: String!
	){
		function {
			core {
				storage {
					register_s3(
						name: $name,
						key: $key,
						secret: $secret,
						region: $region,
						endpoint: $endpoint,
						scope: $scope,
						use_ssl: $use_ssl,
						url_style: $url_style
					) {
						success
						message
					}
				}
			}
		}
	}`, map[string]any{
		"name":      info.Name,
		"key":       info.KeyID,
		"secret":    info.Secret,
		"region":    info.Region,
		"endpoint":  info.Endpoint,
		"scope":     info.Scope,
		"use_ssl":   info.UseSSL,
		"url_style": info.URLStyle,
	})
	if err != nil {
		return err
	}
	defer res.Close()
	if res.Err() != nil {
		return res.Err()
	}
	var op types.OperationResult
	err = res.ScanData("function.core.storage.register_s3", &op)
	if err != nil {
		return err
	}
	if !op.Succeed {
		return errors.New(op.Msg)
	}

	return nil
}

func (n *Node) UnregisterS3(ctx context.Context, name string) error {
	if !n.IsReady() {
		return errors.New("node is not ready")
	}
	res, err := n.c.Query(ctx, `mutation($name: String!){
		function {
			core {
				storage {
					unregister_s3(name: $name) {
						success
						message
					}
				}
			}
		}
	}`, map[string]any{
		"name": name,
	})
	if err != nil {
		return err
	}
	if res.Err() != nil {
		return res.Err()
	}
	defer res.Close()
	var op types.OperationResult
	err = res.ScanData("function.core.storage.unregister_s3", &op)
	if err != nil {
		return err
	}
	if !op.Succeed {
		return errors.New(op.Msg)
	}

	return nil
}

type StorageInfo struct {
	Node     string   `json:"node"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	KeyID    string   `json:"key"`
	Secret   string   `json:"secret"`
	Region   string   `json:"region"`
	Endpoint string   `json:"endpoint"`
	Scope    []string `json:"scope"`
	UseSSL   bool     `json:"use_ssl"`
	URLStyle string   `json:"url_style"`
}

func (n *Node) RegisteredStorages(ctx context.Context) ([]StorageInfo, error) {
	if !n.IsReady() {
		return nil, errNotReady
	}
	res, err := n.c.Query(ctx, `query{
		core {
			storage {
				registered_s3 {
					name
					type
					key
					region
					endpoint
					scope
					use_ssl
					url_style
				}
			}
		}
	}`, nil)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	if res.Err() != nil {
		return nil, res.Err()
	}
	var ss []StorageInfo
	err = res.ScanData("core.storage.registered_s3", &ss)
	if err != nil {
		return nil, err
	}

	return ss, nil
}
