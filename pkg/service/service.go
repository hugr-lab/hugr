package service

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// THe service http handler - expose health check and metrics for prometheus

type Service struct {
	bind string
	mux  *http.ServeMux
	srv  *http.Server
}

func New(bind string) *Service {
	return &Service{
		bind: bind,
		mux:  http.NewServeMux(),
	}
}

func (s *Service) Start(ctx context.Context) error {
	if s.bind == "" {
		return nil
	}
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	s.mux.Handle("/metrics", promhttp.Handler())
	s.srv = &http.Server{
		Addr:    s.bind,
		Handler: s.mux,
	}
	go func() {
		log.Printf("Starting service server on %s", s.bind)
		err := s.srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			log.Println("Service server closed")
			return
		}
		if err != nil {
			log.Printf("Error starting service: %v", err)
		}
	}()
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
