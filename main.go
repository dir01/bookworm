package main

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		log.Fatalf("[main] DB_PATH env var is not set")
	}
	booksDir := os.Getenv("BOOKS_DIR")
	if booksDir == "" {
		log.Fatalf("[main] BOOKS_DIR env var is not set")
	}
	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = ":8080"
		log.Println("[main] BIND_ADDR env var is not set, using default :8080")
	}

	db := sqlx.MustConnect("sqlite3", dbPath)
	store := NewSqliteStore(db)
	svc, err := NewService(booksDir, store)
	if err != nil {
		log.Fatalf("[main] can't create service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigs
		log.Printf("[main] got %s signal, shutting down", s)
		cancel()
	}()

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("[main] can't run service: %v", err)
	}

	go func() {
		if err := svc.Scan(ctx); err != nil {
			log.Fatalf("[main] can't scan dir: %v", err)
		}
	}()

	mux := NewHttpMux(svc)
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Printf("[main] listening on %s", bindAddr)
		if err := srv.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				log.Printf("[main] server closed")
				return
			}
			log.Fatalf("[main] can't listen and serve: %v", err)
		}
	}()

	<-ctx.Done()

	log.Printf("[main] shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("[main] can't shutdown server: %v", err)
	}
}