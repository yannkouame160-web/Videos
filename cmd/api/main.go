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

	// Messages d'information au démarrage : les codes vides désactivent les
	// fonctionnalités correspondantes. Les valeurs des codes ne sont jamais journalisées.
	if cfg.InvitationCode == "" {
		log.Printf("INVITATION_CODE non défini : la création d'applications est désactivée")
	}
	if cfg.AdminCode == "" {
		log.Printf("ADMIN_CODE non défini : l'administration des applications est désactivée")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL est obligatoire")
	}

	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	st := store.NewStore(pool)
	deps := httpapi.Dependencies{
		Store:                    st,
		Utilisateurs:             st,
		InvitationCode:           cfg.InvitationCode,
		AdminCode:                cfg.AdminCode,
		DefaultStorageQuotaBytes: cfg.DefaultStorageQuotaBytes,
		DefaultMaxVideos:         cfg.DefaultMaxVideos,
		DefaultRequestsPerMinute: cfg.DefaultRequestsPerMinute,
	}
	router := httpapi.NewRouter(deps)

	addr := ":" + cfg.HTTPPort
	log.Printf("API démarrée sur le port %s", cfg.HTTPPort)
	log.Fatal(http.ListenAndServe(addr, router))
}
