// Command server runs the rammed-earth roof-beam clearance quality backend.
// It wires the embedded bbolt persistence store, performs startup recovery,
// exposes the versioned JSON API and serves until interrupted. Restarting the
// process with the same DB_PATH reloads every incomplete aggregate, expired
// lease and pending instrument retry.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"rammed-earth-roof-beam-clearance/internal/httpapi"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func main() {
	addr := envOr("ADDR", ":8080")
	dbPath := envOr("DB_PATH", filepath.Join(os.TempDir(), "rammed-earth.db"))

	st, err := store.OpenBoltStore(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	if err := st.Recover(); err != nil {
		log.Fatalf("recovery failed: %v", err)
	}
	log.Printf("store ready at %s", dbPath)

	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(st).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Printf("store close: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
