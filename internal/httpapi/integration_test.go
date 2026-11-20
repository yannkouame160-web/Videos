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

func TestScenarioUtilisateursAvecVraieBase(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	deps := Dependencies{
		Store:                    store.NewStore(pool),
		Utilisateurs:             store.NewStore(pool),
		InvitationCode:           "code-invitation-test",
		AdminCode:                "code-admin-test",
		DefaultStorageQuotaBytes: 1234,
		DefaultMaxVideos:         56,
		DefaultRequestsPerMinute: 78,
	}
	router := NewRouter(deps)

	// Crée les applications A et B.
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App A"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("création de A: code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repA struct {
		ID  string `json:"id_application"`
		Cle string `json:"cle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repA); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}

	rec = postApplication(t, router, "code-invitation-test", `{"nom":"App B"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("création de B: code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repB struct {
		ID  string `json:"id_application"`
		Cle string `json:"cle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repB); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}

	// (a) avec la clé de A : GET /v1/utilisateurs/u1 : 201 rôle "lecteur".
	rec = getUtilisateurAvec(t, router, repA.Cle, "u1")
	if rec.Code != http.StatusCreated {
		t.Fatalf("(a) code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleLecteur {
		t.Errorf("(a) utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleLecteur, id, role)
	}

	// (b) PUT .../u1/role {"role":"administrateur"} : 200.
	rec = putRoleAvec(t, router, repA.Cle, "u1", `{"role":"administrateur"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("(b) code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	// (c) GET : 200 avec rôle "administrateur".
	rec = getUtilisateurAvec(t, router, repA.Cle, "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("(c) code HTTP: attendu 200, obtenu %d", rec.Code)
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleAdministrateur {
		t.Errorf("(c) utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleAdministrateur, id, role)
	}

	// (d) avec la clé de B : 201 rôle "lecteur" (utilisateur distinct, rôle non contaminé).
	rec = getUtilisateurAvec(t, router, repB.Cle, "u1")
	if rec.Code != http.StatusCreated {
		t.Fatalf("(d) code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleLecteur {
		t.Errorf("(d) utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleLecteur, id, role)
	}

	// (e) DELETE /v1/utilisateurs/u1 avec la clé de A : 204.
	rec = deleteAvec(t, router, repA.Cle, "u1", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("(e) code HTTP: attendu 204, obtenu %d", rec.Code)
	}

	// (f) GET avec la clé de A : 201 (recréé).
	rec = getUtilisateurAvec(t, router, repA.Cle, "u1")
	if rec.Code != http.StatusCreated {
		t.Fatalf("(f) code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}

	// (g) GET avec la clé de B : 200, toujours rôle "lecteur".
	rec = getUtilisateurAvec(t, router, repB.Cle, "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("(g) code HTTP: attendu 200, obtenu %d", rec.Code)
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleLecteur {
		t.Errorf("(g) utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleLecteur, id, role)
	}

	// Après mise à jour directe en SQL de creation_auto_utilisateur = false pour A :
	if _, err := pool.Exec(ctx, `UPDATE application SET creation_auto_utilisateur = false WHERE id_application = $1::uuid`, repA.ID); err != nil {
		t.Fatalf("mise à jour creation_auto_utilisateur: %v", err)
	}
	rec = getUtilisateurAvec(t, router, repA.Cle, "utilisateur-inconnu")
	if rec.Code != http.StatusNotFound {
		t.Errorf("code HTTP: attendu 404, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if code := codeErreur(t, rec); code != "utilisateur_introuvable" {
		t.Errorf("code d'erreur: attendu utilisateur_introuvable, obtenu %q", code)
	}

	// Application A désactivée par la route d'administration : GET avec sa clé : 403.
	rec = postAdmin(t, router, repA.ID, "desactiver", "code-admin-test")
	if rec.Code != http.StatusOK {
		t.Fatalf("désactivation de A: code HTTP: attendu 200, obtenu %d", rec.Code)
	}
	rec = getUtilisateurAvec(t, router, repA.Cle, "u1")
	if rec.Code != http.StatusForbidden {
		t.Errorf("code HTTP: attendu 403, obtenu %d (%s)", rec.Code, rec.Body.String())
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
