package config

import "testing"

// quotasParDefaut neutralise les variables de quota pour que chaque test soit indépendant.
func quotasParDefaut(t *testing.T) {
	t.Helper()
	t.Setenv("DEFAULT_QUOTA_STORAGE_BYTES", "")
	t.Setenv("DEFAULT_QUOTA_MAX_VIDEOS", "")
	t.Setenv("DEFAULT_RATE_LIMIT_PER_MINUTE", "")
}

func TestLoadDefaultPort(t *testing.T) {
	quotasParDefaut(t)
	t.Setenv("HTTP_PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue: %v", err)
	}
	if cfg.HTTPPort != "8080" {
		t.Errorf("port par défaut: attendu 8080, obtenu %q", cfg.HTTPPort)
	}
}

func TestLoadCustomPort(t *testing.T) {
	quotasParDefaut(t)
	t.Setenv("HTTP_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue: %v", err)
	}
	if cfg.HTTPPort != "9090" {
		t.Errorf("port personnalisé: attendu 9090, obtenu %q", cfg.HTTPPort)
	}
}

func TestLoadAllVariables(t *testing.T) {
	quotasParDefaut(t)
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/keyavi")
	t.Setenv("INVITATION_CODE", "code-secret")
	t.Setenv("ADMIN_CODE", "admin-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue: %v", err)
	}
	if cfg.HTTPPort != "9090" {
		t.Errorf("HTTPPort: attendu 9090, obtenu %q", cfg.HTTPPort)
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/keyavi" {
		t.Errorf("DatabaseURL: valeur inattendue %q", cfg.DatabaseURL)
	}
	if cfg.InvitationCode != "code-secret" {
		t.Errorf("InvitationCode: valeur inattendue %q", cfg.InvitationCode)
	}
	if cfg.AdminCode != "admin-secret" {
		t.Errorf("AdminCode: valeur inattendue %q", cfg.AdminCode)
	}
}

func TestLoadValeursParDefaut(t *testing.T) {
	quotasParDefaut(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue: %v", err)
	}
	if cfg.DefaultStorageQuotaBytes != 10737418240 {
		t.Errorf("stockage: attendu 10737418240, obtenu %d", cfg.DefaultStorageQuotaBytes)
	}
	if cfg.DefaultMaxVideos != 1000 {
		t.Errorf("max vidéos: attendu 1000, obtenu %d", cfg.DefaultMaxVideos)
	}
	if cfg.DefaultRequestsPerMinute != 600 {
		t.Errorf("requêtes/min: attendu 600, obtenu %d", cfg.DefaultRequestsPerMinute)
	}
}

func TestLoadQuotasPersonnalises(t *testing.T) {
	quotasParDefaut(t)
	t.Setenv("DEFAULT_QUOTA_STORAGE_BYTES", "2048")
	t.Setenv("DEFAULT_QUOTA_MAX_VIDEOS", "42")
	t.Setenv("DEFAULT_RATE_LIMIT_PER_MINUTE", "7")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue: %v", err)
	}
	if cfg.DefaultStorageQuotaBytes != 2048 {
		t.Errorf("stockage: attendu 2048, obtenu %d", cfg.DefaultStorageQuotaBytes)
	}
	if cfg.DefaultMaxVideos != 42 {
		t.Errorf("max vidéos: attendu 42, obtenu %d", cfg.DefaultMaxVideos)
	}
	if cfg.DefaultRequestsPerMinute != 7 {
		t.Errorf("requêtes/min: attendu 7, obtenu %d", cfg.DefaultRequestsPerMinute)
	}
}

func TestLoadQuotasInvalides(t *testing.T) {
	cas := []struct {
		nom  string
		vals map[string]string
	}{
		{"texte", map[string]string{"DEFAULT_QUOTA_STORAGE_BYTES": "beaucoup"}},
		{"negatif", map[string]string{"DEFAULT_QUOTA_MAX_VIDEOS": "-1"}},
		{"zero", map[string]string{"DEFAULT_RATE_LIMIT_PER_MINUTE": "0"}},
	}

	for _, tc := range cas {
		t.Run(tc.nom, func(t *testing.T) {
			quotasParDefaut(t)
			for nom, valeur := range tc.vals {
				t.Setenv(nom, valeur)
			}

			if _, err := Load(); err == nil {
				t.Errorf("Load: une erreur était attendue pour %v", tc.vals)
			}
		})
	}
}
