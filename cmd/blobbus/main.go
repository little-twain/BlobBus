package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/littletwain/blobbus/internal/blobbus"
)

func main() {
	cfg, err := blobbus.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := blobbus.NewStore(cfg)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           blobbus.NewHandler(store),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("blobbus listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
