package config

import (
	"strings"
	"testing"
)

// quotasParDefaut neutralise les variables de quota pour que chaque test soit indépendant.
func quotasParDefaut(t *testing.T) {
	t.Helper()
	t.Setenv("DEFAULT_QUOTA_STORAGE_BYTES", "")
	t.Setenv("DEFAULT_QUOTA_MAX_VIDEOS", "")
	t.Setenv("DEFAULT_RATE_LIMIT_PER_MINUTE", "")
}

// codesParDefaut vide les codes et l'URL pour que chaque test soit indépendant du fichier .env.
func codesParDefaut(t *testing.T) {
	t.Helper()
	t.Setenv("INVITATION_CODE", "")
	t.Setenv("ADMIN_CODE", "")
	t.Setenv("DATABASE_URL", "")
}

func TestLoadDefaultPort(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)
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
	codesParDefaut(t)
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
	codesParDefaut(t)
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/keyavi")
	t.Setenv("INVITATION_CODE", "code-invitation-01")
	t.Setenv("ADMIN_CODE", "code-admin-01-01")

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
	if cfg.InvitationCode != "code-invitation-01" {
		t.Errorf("InvitationCode: valeur inattendue %q", cfg.InvitationCode)
	}
	if cfg.AdminCode != "code-admin-01-01" {
		t.Errorf("AdminCode: valeur inattendue %q", cfg.AdminCode)
	}
}

func TestLoadValeursParDefaut(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)

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
	codesParDefaut(t)
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
			codesParDefaut(t)
			for nom, valeur := range tc.vals {
				t.Setenv(nom, valeur)
			}

			if _, err := Load(); err == nil {
				t.Errorf("Load: une erreur était attendue pour %v", tc.vals)
			}
		})
	}
}

func TestLoadInvitationCodeTropCourt(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)
	t.Setenv("INVITATION_CODE", "court")

	_, err := Load()
	if err == nil {
		t.Fatal("une erreur était attendue pour un code d'invitation trop court")
	}
	const attendu = "INVITATION_CODE doit contenir au moins 16 caractères"
	if got := err.Error(); got != attendu {
		t.Errorf("message d'erreur: attendu %q, obtenu %q", attendu, got)
	}
	if strings.Contains(err.Error(), "court") {
		t.Error("le message d'erreur ne doit pas contenir la valeur du code")
	}
}

func TestLoadAdminCodeTropCourt(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)
	t.Setenv("ADMIN_CODE", "admin-x")

	_, err := Load()
	if err == nil {
		t.Fatal("une erreur était attendue pour un code d'administration trop court")
	}
	const attendu = "ADMIN_CODE doit contenir au moins 16 caractères"
	if got := err.Error(); got != attendu {
		t.Errorf("message d'erreur: attendu %q, obtenu %q", attendu, got)
	}
	if strings.Contains(err.Error(), "admin-x") {
		t.Error("le message d'erreur ne doit pas contenir la valeur du code")
	}
}

func TestLoadCodesIdentiques(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)
	code := "code-identique-01"
	t.Setenv("INVITATION_CODE", code)
	t.Setenv("ADMIN_CODE", code)

	_, err := Load()
	if err == nil {
		t.Fatal("une erreur était attendue pour des codes identiques")
	}
	const attendu = "INVITATION_CODE et ADMIN_CODE doivent être différents"
	if got := err.Error(); got != attendu {
		t.Errorf("message d'erreur: attendu %q, obtenu %q", attendu, got)
	}
	if strings.Contains(err.Error(), code) {
		t.Error("le message d'erreur ne doit pas contenir la valeur du code")
	}
}

func TestLoadCodesVidesAcceptes(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue avec les codes vides: %v", err)
	}
	if cfg.InvitationCode != "" {
		t.Errorf("InvitationCode: attendu vide, obtenu %q", cfg.InvitationCode)
	}
	if cfg.AdminCode != "" {
		t.Errorf("AdminCode: attendu vide, obtenu %q", cfg.AdminCode)
	}
}

func TestLoadUnSeulCodeDefiniAccepte(t *testing.T) {
	quotasParDefaut(t)
	codesParDefaut(t)

	// Seul le code d'invitation défini (d'au moins 16 caractères).
	t.Setenv("INVITATION_CODE", "code-invitation-01")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue avec le code d'invitation seul: %v", err)
	}
	if cfg.InvitationCode != "code-invitation-01" || cfg.AdminCode != "" {
		t.Errorf("codes: attendus (%q, vide), obtenus (%q, %q)", "code-invitation-01", cfg.InvitationCode, cfg.AdminCode)
	}

	// Seul le code d'administration défini (d'au moins 16 caractères).
	t.Setenv("INVITATION_CODE", "")
	t.Setenv("ADMIN_CODE", "code-admin-01-01")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: erreur inattendue avec le code d'administration seul: %v", err)
	}
	if cfg.InvitationCode != "" || cfg.AdminCode != "code-admin-01-01" {
		t.Errorf("codes: attendus (vide, %q), obtenus (%q, %q)", "code-admin-01-01", cfg.InvitationCode, cfg.AdminCode)
	}
}
