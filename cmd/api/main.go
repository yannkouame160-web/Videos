package main

import (
	"context"
	"log"
	"net/http"

	"keyavi-videos/internal/config"
	"keyavi-videos/internal/db"
	"keyavi-videos/internal/httpapi"
	"keyavi-videos/internal/store"
)

func main() {
	// La configuration peut refuser des quotas invalides : on s'arrête alors.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL est obligatoire")
	}

	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	deps := httpapi.Dependencies{
		Store:                    store.NewStore(pool),
		InvitationCode:           cfg.InvitationCode,
		DefaultStorageQuotaBytes: cfg.DefaultStorageQuotaBytes,
		DefaultMaxVideos:         cfg.DefaultMaxVideos,
		DefaultRequestsPerMinute: cfg.DefaultRequestsPerMinute,
	}
	router := httpapi.NewRouter(deps)

	addr := ":" + cfg.HTTPPort
	log.Printf("API démarrée sur le port %s", cfg.HTTPPort)
	log.Fatal(http.ListenAndServe(addr, router))
}
