package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"keyavi-videos/internal/store"
	"keyavi-videos/internal/testdb"
)

func TestCreationApplicationAvecVraieBase(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	deps := Dependencies{
		Store:                    store.NewStore(pool),
		InvitationCode:           "code-invitation-test",
		DefaultStorageQuotaBytes: 1234,
		DefaultMaxVideos:         56,
		DefaultRequestsPerMinute: 78,
	}
	router := NewRouter(deps)

	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App BoutEnBout"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	var rep struct {
		ID  string `json:"id_application"`
		Nom string `json:"nom"`
		Cle string `json:"cle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if !strings.HasPrefix(rep.Cle, "kvk_") {
		t.Errorf("la clé doit commencer par kvk_, obtenu %q", rep.Cle)
	}

	var stockage int64
	var videos, requetes int
	err := pool.QueryRow(ctx, `
		SELECT plafond_stockage_octets, plafond_nb_videos, limite_requetes_par_minute
		FROM application WHERE id_application = $1::uuid`, rep.ID,
	).Scan(&stockage, &videos, &requetes)
	if err != nil {
		t.Fatalf("l'application n'existe pas en base: %v", err)
	}
	if stockage != 1234 || videos != 56 || requetes != 78 {
		t.Errorf("plafonds en base incorrects: %d, %d, %d", stockage, videos, requetes)
	}

	rec2 := postApplication(t, router, "code-invitation-test", `{"nom":"App BoutEnBout"}`)
	if rec2.Code != http.StatusConflict {
		t.Errorf("code HTTP du doublon: attendu 409, obtenu %d (%s)", rec2.Code, rec2.Body.String())
	}
	if code := codeErreur(t, rec2); code != "nom_deja_utilise" {
		t.Errorf("code d'erreur: attendu nom_deja_utilise, obtenu %q", code)
	}
}

func TestScenarioCompletVraieBase(t *testing.T) {
	pool := testdb.Open(t)

	deps := Dependencies{
		Store:                    store.NewStore(pool),
		InvitationCode:           "code-invitation-test",
		AdminCode:                "code-admin-test",
		DefaultStorageQuotaBytes: 1234,
		DefaultMaxVideos:         56,
		DefaultRequestsPerMinute: 78,
	}
	router := NewRouter(deps)

	// (1) POST /v1/applications crée l'application A.
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App A"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("(1) code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repA struct {
		ID  string `json:"id_application"`
		Cle string `json:"cle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repA); err != nil {
		t.Fatalf("(1) réponse JSON invalide: %v", err)
	}
	if repA.ID == "" || repA.Cle == "" {
		t.Fatalf("(1) id ou clé manquant dans %q", rec.Body.String())
	}

	// (2) GET /v1/application avec la clé de A : 200 et le nom de A.
	rec = getApplication(t, router, repA.Cle)
	if rec.Code != http.StatusOK {
		t.Fatalf("(2) code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repNomA struct {
		Nom string `json:"nom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repNomA); err != nil {
		t.Fatalf("(2) réponse JSON invalide: %v", err)
	}
	if repNomA.Nom != "App A" {
		t.Errorf("(2) nom: attendu %q, obtenu %q", "App A", repNomA.Nom)
	}

	// (3) POST .../desactiver avec le bon X-Admin-Code : 200.
	rec = postAdmin(t, router, repA.ID, "desactiver", "code-admin-test")
	if rec.Code != http.StatusOK {
		t.Fatalf("(3) code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	// (4) GET /v1/application avec la même clé : 403.
	rec = getApplication(t, router, repA.Cle)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("(4) code HTTP: attendu 403, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	// (5) POST .../activer : 200.
	rec = postAdmin(t, router, repA.ID, "activer", "code-admin-test")
	if rec.Code != http.StatusOK {
		t.Fatalf("(5) code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	// (6) GET /v1/application : 200 de nouveau.
	rec = getApplication(t, router, repA.Cle)
	if rec.Code != http.StatusOK {
		t.Fatalf("(6) code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	// Crée une application B.
	rec = postApplication(t, router, "code-invitation-test", `{"nom":"App B"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("création de B: code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repB struct {
		ID  string `json:"id_application"`
		Cle string `json:"cle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repB); err != nil {
		t.Fatalf("réponse JSON de B invalide: %v", err)
	}

	// La clé de A renvoie toujours le nom de A ; la clé de B le nom de B (jamais mélangés).
	rec = getApplication(t, router, repA.Cle)
	var repNomA2 struct {
		Nom string `json:"nom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repNomA2); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if repNomA2.Nom != "App A" {
		t.Errorf("clé de A: nom attendu %q, obtenu %q", "App A", repNomA2.Nom)
	}

	rec = getApplication(t, router, repB.Cle)
	var repNomB struct {
		Nom string `json:"nom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repNomB); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if repNomB.Nom != "App B" {
		t.Errorf("clé de B: nom attendu %q, obtenu %q", "App B", repNomB.Nom)
	}

	// Une fausse clé (bonne forme kvk_ + 64 hexadécimaux, inconnue) renvoie 401.
	rec = getApplication(t, router, "kvk_"+strings.Repeat("ab", 32))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("fausse clé: code HTTP: attendu 401, obtenu %d", rec.Code)
	}

	// Désactiver A ne change PAS l'état de B : GET avec la clé de B toujours 200.
	rec = postAdmin(t, router, repA.ID, "desactiver", "code-admin-test")
	if rec.Code != http.StatusOK {
		t.Fatalf("désactiver A: code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	rec = getApplication(t, router, repB.Cle)
	if rec.Code != http.StatusOK {
		t.Errorf("état de B : attendu 200, obtenu %d", rec.Code)
	}
}
