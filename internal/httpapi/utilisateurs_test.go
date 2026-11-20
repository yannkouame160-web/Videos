package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"keyavi-videos/internal/store"
)

// cleTester respecte la forme attendue par le portier (préfixe kvk_ + 68
// caractères au total).
const cleTester = "kvk_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func depsAvecUtilisateurs(f *fauxStore) Dependencies {
	deps := depsAvecCode(f)
	deps.Utilisateurs = f
	return deps
}

func getUtilisateurAvec(t *testing.T, router http.Handler, cle, idExterne string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/utilisateurs/"+idExterne, nil)
	req.Header.Set("Authorization", "Bearer "+cle)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func putRoleAvec(t *testing.T, router http.Handler, cle, idExterne, corps string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/v1/utilisateurs/"+idExterne+"/role", strings.NewReader(corps))
	req.Header.Set("Authorization", "Bearer "+cle)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func deleteAvec(t *testing.T, router http.Handler, cle, idExterne, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/v1/utilisateurs/" + idExterne
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+cle)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func reponseUtilisateur(t *testing.T, rec *httptest.ResponseRecorder) (id, role string) {
	t.Helper()
	var rep struct {
		IDExterne              string    `json:"id_externe"`
		Role                   string    `json:"role"`
		DatePremiereApparition time.Time `json:"date_premiere_apparition"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("réponse JSON invalide: %v (%q)", err, rec.Body.String())
	}
	return rep.IDExterne, rep.Role
}

func appUtilisateurTest() store.Application {
	return store.Application{ID: "app-1", Nom: "App", EstActive: true, CreationAutoUtilisateur: true}
}

func TestPortierSansEnTeteSurUtilisateurs(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, "", "u1")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET: code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("GET: code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	rec = putRoleAvec(t, router, "", "u1", `{"role":"lecteur"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("PUT: code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("PUT: code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	rec = deleteAvec(t, router, "", "u1", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("DELETE: code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("DELETE: code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	if f.idAppUserRecu != "" {
		t.Error("le store ne doit pas être appelé sans clé")
	}
}

func TestPortierApplicationDesactiveeSurUtilisateurs(t *testing.T) {
	f := &fauxStore{byKey: store.Application{ID: "app-1", EstActive: false}}
	router := NewRouter(depsAvecUtilisateurs(f))

	for nom, rec := range map[string]*httptest.ResponseRecorder{
		"GET":    getUtilisateurAvec(t, router, cleTester, "u1"),
		"PUT":    putRoleAvec(t, router, cleTester, "u1", `{"role":"lecteur"}`),
		"DELETE": deleteAvec(t, router, cleTester, "u1", ""),
	} {
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: code HTTP: attendu 403, obtenu %d", nom, rec.Code)
		}
		if code := codeErreur(t, rec); code != "application_desactivee" {
			t.Errorf("%s: code d'erreur: attendu application_desactivee, obtenu %q", nom, code)
		}
	}
}

func TestGetUtilisateurExistant(t *testing.T) {
	f := &fauxStore{
		byKey: appUtilisateurTest(),
		user:  store.Utilisateur{IDExterne: "u1", Role: store.RoleLecteur, DatePremiereApparition: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)},
	}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, cleTester, "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleLecteur {
		t.Errorf("utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleLecteur, id, role)
	}
	if f.idAppUserRecu != "app-1" {
		t.Errorf("le store doit recevoir l'id de l'application authentifiée (%q), obtenu %q", "app-1", f.idAppUserRecu)
	}
	if f.idExterneUserRecu != "u1" {
		t.Errorf("le store doit recevoir l'id externe %q, obtenu %q", "u1", f.idExterneUserRecu)
	}
	if f.callsByKey != 1 {
		t.Errorf("le portier doit être appelé une fois, obtenu %d", f.callsByKey)
	}
}

func TestGetUtilisateurCreation(t *testing.T) {
	f := &fauxStore{
		byKey: appUtilisateurTest(),
		user:  store.Utilisateur{IDExterne: "u1", Role: store.RoleLecteur},
		cree:  true,
	}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, cleTester, "u1")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleLecteur {
		t.Errorf("utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleLecteur, id, role)
	}
	if f.idAppUserRecu != "app-1" {
		t.Errorf("le store doit recevoir l'id de l'application authentifiée (%q), obtenu %q", "app-1", f.idAppUserRecu)
	}
}

func TestGetUtilisateurAbsentSansCreation(t *testing.T) {
	f := &fauxStore{
		byKey:   store.Application{ID: "app-1", EstActive: true, CreationAutoUtilisateur: false},
		errUser: store.ErrUtilisateurIntrouvable,
	}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, cleTester, "u1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code HTTP: attendu 404, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if code := codeErreur(t, rec); code != "utilisateur_introuvable" {
		t.Errorf("code d'erreur: attendu utilisateur_introuvable, obtenu %q", code)
	}
	if f.idAppUserRecu != "app-1" {
		t.Errorf("le store doit recevoir l'id de l'application authentifiée (%q), obtenu %q", "app-1", f.idAppUserRecu)
	}
}

func TestGetUtilisateurIdentifiantTropLong(t *testing.T) {
	f := &fauxStore{byKey: appUtilisateurTest()}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, cleTester, strings.Repeat("a", 129))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "identifiant_invalide" {
		t.Errorf("code d'erreur: attendu identifiant_invalide, obtenu %q", code)
	}
	if f.idExterneUserRecu != "" {
		t.Error("le store ne doit pas être appelé")
	}
}

func TestGetUtilisateurIdentifiantControle(t *testing.T) {
	f := &fauxStore{byKey: appUtilisateurTest()}
	router := NewRouter(depsAvecUtilisateurs(f))

	req := httptest.NewRequest(http.MethodGet, "/v1/utilisateurs/u%01", nil)
	req.Header.Set("Authorization", "Bearer "+cleTester)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "identifiant_invalide" {
		t.Errorf("code d'erreur: attendu identifiant_invalide, obtenu %q", code)
	}
	if f.idExterneUserRecu != "" {
		t.Error("le store ne doit pas être appelé")
	}
}

func TestPutRoleUtilisateur(t *testing.T) {
	f := &fauxStore{
		byKey: appUtilisateurTest(),
		user:  store.Utilisateur{IDExterne: "u1", Role: store.RoleLecteur},
	}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := putRoleAvec(t, router, cleTester, "u1", `{"role":"administrateur"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if f.roleRecu != store.RoleAdministrateur {
		t.Errorf("role transmis au store: attendu %q, obtenu %q", store.RoleAdministrateur, f.roleRecu)
	}
	if id, role := reponseUtilisateur(t, rec); id != "u1" || role != store.RoleAdministrateur {
		t.Errorf("utilisateur: attendu (%q, %q), obtenu (%q, %q)", "u1", store.RoleAdministrateur, id, role)
	}
}

func TestPutRoleUtilisateurInvalide(t *testing.T) {
	cas := []struct {
		nom   string
		corps string
		code  string
	}{
		{"role invalide", `{"role":"superadmin"}`, "role_invalide"},
		{"role absent", `{}`, "role_invalide"},
		{"champ inconnu", `{"role":"lecteur","inconnu":1}`, "corps_invalide"},
	}
	for _, tc := range cas {
		t.Run(tc.nom, func(t *testing.T) {
			f := &fauxStore{byKey: appUtilisateurTest()}
			router := NewRouter(depsAvecUtilisateurs(f))
			rec := putRoleAvec(t, router, cleTester, "u1", tc.corps)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code HTTP: attendu 400, obtenu %d", rec.Code)
			}
			if code := codeErreur(t, rec); code != tc.code {
				t.Errorf("code d'erreur: attendu %q, obtenu %q", tc.code, code)
			}
		})
	}
}

func TestDeleteUtilisateurParams(t *testing.T) {
	f := &fauxStore{byKey: appUtilisateurTest()}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := deleteAvec(t, router, cleTester, "u1", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("sans paramètre: code HTTP: attendu 204, obtenu %d", rec.Code)
	}
	if f.supprimerVideosRecu {
		t.Error("supprimerVideos doit être false par défaut")
	}
	if f.idExterneUserRecu != "u1" {
		t.Errorf("le store doit recevoir l'id externe %q, obtenu %q", "u1", f.idExterneUserRecu)
	}
	if f.idAppUserRecu != "app-1" {
		t.Errorf("le store doit recevoir l'id de l'application authentifiée (%q), obtenu %q", "app-1", f.idAppUserRecu)
	}

	rec = deleteAvec(t, router, cleTester, "u1", "supprimer_videos=true")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("?supprimer_videos=true: code HTTP: attendu 204, obtenu %d", rec.Code)
	}
	if !f.supprimerVideosRecu {
		t.Error("supprimerVideos doit être true avec ?supprimer_videos=true")
	}

	rec = deleteAvec(t, router, cleTester, "u1", "supprimer_videos=oui")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("?supprimer_videos=oui: code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "parametre_invalide" {
		t.Errorf("code d'erreur: attendu parametre_invalide, obtenu %q", code)
	}
}

func TestErreurTechniqueUtilisateurs(t *testing.T) {
	const detail = "ERREUR_UTILISATEURS_SECRETE_777"
	f := &fauxStore{
		byKey:   appUtilisateurTest(),
		errUser: errors.New(detail),
	}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, cleTester, "u1")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET: code HTTP: attendu 500, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "erreur_interne" {
		t.Errorf("GET: code d'erreur: attendu erreur_interne, obtenu %q", code)
	}
	if strings.Contains(rec.Body.String(), detail) {
		t.Error("GET: le détail technique ne doit pas apparaître dans la réponse")
	}

	rec = putRoleAvec(t, router, cleTester, "u1", `{"role":"lecteur"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("PUT: code HTTP: attendu 500, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "erreur_interne" {
		t.Errorf("PUT: code d'erreur: attendu erreur_interne, obtenu %q", code)
	}
	if strings.Contains(rec.Body.String(), detail) {
		t.Error("PUT: le détail technique ne doit pas apparaître dans la réponse")
	}

	rec = deleteAvec(t, router, cleTester, "u1", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DELETE: code HTTP: attendu 500, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "erreur_interne" {
		t.Errorf("DELETE: code d'erreur: attendu erreur_interne, obtenu %q", code)
	}
	if strings.Contains(rec.Body.String(), detail) {
		t.Error("DELETE: le détail technique ne doit pas apparaître dans la réponse")
	}
}

// Interface utilisée par les méthodes de fauxStore pour satisfaire UtilisateurStore.
var _ UtilisateurStore = (*fauxStore)(nil)

// Interface utilisée par les méthodes de fauxStore pour satisfaire ApplicationStore.
var _ ApplicationStore = (*fauxStore)(nil)

// Interface utilisée pour vérifier que Store (magasin réel) implémente UtilisateurStore.
var _ UtilisateurStore = (*store.Store)(nil)

func TestUtilisateurIdentifiantUTF8Invalide(t *testing.T) {
	f := &fauxStore{byKey: store.Application{ID: "app-1", EstActive: true}}
	router := NewRouter(depsAvecUtilisateurs(f))

	rec := getUtilisateurAvec(t, router, cleTester, "\xff")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "identifiant_invalide" {
		t.Errorf("code d'erreur: attendu identifiant_invalide, obtenu %q", code)
	}
	if f.idAppUserRecu != "" {
		t.Error("le store utilisateur ne doit pas etre appele")
	}
}
