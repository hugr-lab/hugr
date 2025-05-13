package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/hugr-lab/hugr/pkg/cluster"
	"github.com/hugr-lab/hugr/pkg/service"
)

// The hugr cluster management node.
// This service provides node register management and data source management sync methods.

func main() {
	conf := loadConfig()
	err := conf.parseAuth()
	if err != nil {
		log.Println("Auth configuration error:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	c := cluster.New(conf.Cluster)
	err = c.Init()
	if err != nil {
		log.Println("Cluster initialization error:", err)
		os.Exit(1)
	}
	defer c.Stop()

	srv := &http.Server{
		Addr:    conf.Bind,
		Handler: c,
	}
	go func() {
		log.Println("Starting server on ", conf.Bind)
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			log.Println("Server closed")
			return
		}
		if err != nil {
			log.Println("Server error:", err)
			os.Exit(1)
		}
	}()
	svc := service.New(conf.ServiceBind)
	err = svc.Start(ctx)
	if err != nil {
		log.Println("Services endpoint server start error:", err)
	}
	<-ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	if err != nil {
		log.Println("Server shutdown error:", err)
		os.Exit(1)
	}
	err = svc.Stop(ctx)
	if err != nil {
		log.Println("Service shutdown error:", err)
		os.Exit(1)
	}
	log.Println("Server shutdown")
}
