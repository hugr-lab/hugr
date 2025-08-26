package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/hugr-lab/hugr/pkg/auth"
	"github.com/hugr-lab/hugr/pkg/cors"
	coredb "github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/core-db"
	"github.com/hugr-lab/query-engine/pkg/data-sources/sources/runtime/storage"
)

type Config struct {
	Secret  string
	Version string
	Timeout time.Duration
	Check   time.Duration

	Node NodeCommonConfig
}

type NodeCommonConfig struct {
	EnableAdminUI    bool                 `json:"enable_admin_ui"`
	AdminUIFetchPath string               `json:"admin_ui_fetch_path"`
	DebugMode        bool                 `json:"debug_mode"`
	Cors             cors.Config          `json:"cors"`
	CoreDB           coredb.Config        `json:"core_db"`
	Auth             auth.ProvidersConfig `json:"auth"`
}

type Service struct {
	version    string
	secret     string
	timeout    time.Duration
	check      time.Duration
	nodeConfig NodeCommonConfig

	router *http.ServeMux

	mu    sync.Mutex
	nodes map[string]*Node
}

func New(config Config) *Service {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Check == 0 {
		config.Check = 60 * time.Second
	}
	return &Service{
		version:    config.Version,
		secret:     config.Secret,
		timeout:    config.Timeout,
		check:      config.Check,
		nodeConfig: config.Node,
		router:     http.NewServeMux(),
		nodes:      make(map[string]*Node),
	}
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, node := range s.nodes {
		node.Stop()
	}
}

func (s *Service) Init() error {
	s.router.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"version":"` + s.version + `"}`))
	})
	s.router.HandleFunc("/node", s.nodeHandler)
	s.router.HandleFunc("/data-source/{name}/status", s.dataSourceStatusHandler)
	s.router.HandleFunc("/data-source/{name}/load", s.loadDataSourceHandler)
	s.router.HandleFunc("/data-source/{name}/unload", s.unloadDataSourceHandler)
	s.router.HandleFunc("/storages", s.registeredStoragesHandler)
	s.router.HandleFunc("/storages/{name}", s.unregisterObjectStorageHandler)

	return nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-hugr-secret") != s.secret {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}
	s.router.ServeHTTP(w, r)
}

type NodeStatus struct {
	Name     string    `json:"name"`
	Url      string    `json:"url"`
	Version  string    `json:"version"`
	Error    string    `json:"error"`
	LastSeen time.Time `json:"last_seen"`
	IsReady  bool      `json:"ready"`
}

// registers a new node in the cluster
// accepts a GET request with the node URL
func (s *Service) nodeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()

		var nodes []NodeStatus
		for name, node := range s.nodes {
			nodes = append(nodes, NodeStatus{
				Name:     name,
				Url:      node.url,
				Version:  node.Version(),
				Error:    node.Error(),
				LastSeen: node.LastSeen(),
				IsReady:  node.IsReady(),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(nodes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	case http.MethodPost:
		nodeURL := r.URL.Query().Get("url")
		if nodeURL == "" {
			http.Error(w, "node_url is required", http.StatusBadRequest)
			return
		}
		nodeName := r.URL.Query().Get("name") // optional
		if nodeName == "" {
			nodeName = nodeURL
		}
		nodeVersion := r.URL.Query().Get("version") // optional

		s.mu.Lock()
		defer s.mu.Unlock()

		if node, exists := s.nodes[nodeURL]; exists {
			node.Stop()
			delete(s.nodes, nodeURL)
			return
		}
		node, err := NewNode(r.Context(), NodeConfig{
			URL:            nodeURL,
			Secret:         s.secret,
			Timeout:        s.timeout,
			Interval:       s.check,
			ClusterVersion: s.version,
			Version:        nodeVersion,
		})
		if err != nil {
			log.Printf("ERR: failed to create node %s: %v", nodeName, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go node.Start(context.Background())
		s.nodes[nodeName] = node
		log.Printf("INFO: node %s registered with URL %s", nodeName, nodeURL)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(s.nodeConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case http.MethodDelete:
		nn := r.URL.Query().Get("name")
		if nn == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		node, exists := s.nodes[nn]
		if !exists {
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}
		node.Stop()
		delete(s.nodes, nn)
		log.Printf("Node %s unregistered", nn)
		w.WriteHeader(http.StatusOK)
	}
}

type DataSourceStatus struct {
	Node   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

func (s *Service) dataSourceStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(len(s.nodes))*s.timeout)
	defer cancel()
	var ss []DataSourceStatus
	for nn, node := range s.nodes {
		s, err := node.DataSourceStatus(ctx, name)
		if err != nil {
			ss = append(ss, DataSourceStatus{
				Node:   nn,
				Error:  err.Error(),
				Status: "error",
			})
			continue
		}
		ss = append(ss, DataSourceStatus{
			Node:   nn,
			Status: s,
		})
	}
	err := json.NewEncoder(w).Encode(ss)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
}

func (s *Service) loadDataSourceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*s.timeout)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	// load data source
	wg := sync.WaitGroup{}
	wg.Add(len(s.nodes))
	errCh := make(chan error)
	for nn, node := range s.nodes {
		node := node
		nn := nn
		go func() {
			defer wg.Done()
			err := node.LoadDataSource(ctx, name)
			if err != nil {
				log.Printf("ERR: node: %s: failed to load data source on node %s: %v", nn, name, err)
				errCh <- fmt.Errorf("failed to load data source on node %s: %w", nn, err)
			}
			log.Printf("INFO: node: %s: data source %s loaded successfully", nn, name)
		}()
	}
	var errMsg string
	go func() {
		for err := range errCh {
			if errMsg != "" {
				errMsg += ";\n"
			}
			errMsg += err.Error()
		}
	}()
	wg.Wait()
	close(errCh)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":"true", "message":"Data source loaded successfully"}`))

}

func (s *Service) unloadDataSourceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*s.timeout)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	// unload data source
	wg := sync.WaitGroup{}
	wg.Add(len(s.nodes))
	errCh := make(chan error)
	for nn, node := range s.nodes {
		go func() {
			defer wg.Done()
			err := node.UnloadDataSource(ctx, name)
			if err != nil {
				log.Printf("ERR: node: %s: failed to unload data source on node %s: %v", nn, name, err)
				errCh <- fmt.Errorf("failed to unload data source on node %s: %w", nn, err)
			}
			log.Printf("INFO: node: %s: data source %s unloaded successfully", nn, name)
		}()
	}
	var errMsg string
	go func() {
		for err := range errCh {
			if errMsg != "" {
				errMsg += ";\n"
			}
			errMsg += err.Error()
		}
	}()
	wg.Wait()
	close(errCh)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	log.Printf("Data source %s unloaded successfully", name)
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":"true", "message":"Data source unloaded successfully"}`))
}

func (s *Service) registerObjectStorageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ds storage.SecretInfo
	if err := json.NewDecoder(r.Body).Decode(&ds); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*s.timeout)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(len(s.nodes))
	errCh := make(chan error)
	for nn, node := range s.nodes {
		node := node
		nn := nn
		go func() {
			defer wg.Done()
			err := node.RegisterObjectStorage(ctx, ds)
			if err != nil {
				log.Printf("ERR: node: %s: failed to register Object storage %s: %v", nn, ds.Scope, err)
				errCh <- fmt.Errorf("failed to register Object storage %s on node %s: %w", ds.Scope, nn, err)
			}
			log.Printf("INFO: node: %s: Object storage %s registered successfully", nn, ds.Scope)
		}()
	}
	var errMsg string
	go func() {
		for err := range errCh {
			if errMsg != "" {
				errMsg += ";\n"
			}
			errMsg += err.Error()
		}
	}()
	wg.Wait()
	close(errCh)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":"true", "message":"Data source registered successfully"}`))
}

func (s *Service) unregisterObjectStorageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*s.timeout)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(len(s.nodes))
	errCh := make(chan error)
	for nn, node := range s.nodes {
		node := node
		nn := nn
		go func() {
			defer wg.Done()
			err := node.UnregisterObjectStorage(ctx, name)
			if err != nil {
				log.Printf("ERR: node: %s:failed to unregister Object storage: %v", nn, name)
				errCh <- fmt.Errorf("failed to unregister Object storage %s on node %s: %w", name, nn, err)
			}
			log.Printf("INFO: node: %s: Object storage %s registered successfully", nn, name)
		}()
	}
	var errMsg string
	go func() {
		for err := range errCh {
			if errMsg != "" {
				errMsg += ";\n"
			}
			errMsg += err.Error()
		}
	}()
	wg.Wait()
	close(errCh)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":"true", "message":"S3 storage delete successfully"}`))
}

func (s *Service) registeredStoragesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.registerObjectStorageHandler(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(len(s.nodes))*s.timeout)
	defer cancel()

	var storages []StorageInfo
	for name, node := range s.nodes {
		storage, err := node.RegisteredStorages(ctx)
		if errors.Is(err, errNotReady) {
			continue
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, s := range storage {
			s.Node = name
			storages = append(storages, s)
		}
	}
	err := json.NewEncoder(w).Encode(storages)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
}
