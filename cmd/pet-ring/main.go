package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pet-ring/internal/httpapi"
	"pet-ring/internal/store"
	webui "pet-ring/web"
)

func main() {
	databasePath := envOrDefault("PET_RING_DB", "./work/pet-ring.db")
	address := envOrDefault("PET_RING_ADDR", ":8080")
	salt := os.Getenv("PET_RING_DEVICE_SALT")
	if salt == "" {
		salt = "development-only-change-me"
		log.Print("warning: PET_RING_DEVICE_SALT is not set; using development value")
	}

	db, err := store.Open(databasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	handler := newHandler(httpapi.New(db, httpapi.Options{DeviceSalt: salt}))
	server := &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("pet-ring listening on %s", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func newHandler(apiHandler http.Handler) http.Handler {
	assets, err := fs.Sub(webui.Assets, ".")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(assets)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			apiHandler.ServeHTTP(response, request)
			return
		}
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path == "" {
			files.ServeHTTP(response, request)
			return
		}
		if _, err := fs.Stat(assets, path); err == nil {
			files.ServeHTTP(response, request)
			return
		}
		if !strings.Contains(path, ".") {
			clone := request.Clone(request.Context())
			clone.URL.Path = "/"
			files.ServeHTTP(response, clone)
			return
		}
		http.NotFound(response, request)
	})
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
