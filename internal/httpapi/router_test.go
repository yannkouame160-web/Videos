package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keyavi-videos/internal/apikey"
	"keyavi-videos/internal/store"
)

func TestSante(t *testing.T) {
	router := NewRouter(Dependencies{})

	req := httptest.NewRequest(http.MethodGet, "/sante", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("code HTTP: attendu 200, obtenu %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("corps: attendu %q, obtenu %q", "ok", rec.Body.String())
	}
}

// fauxStore enregistre ce qu'il reçoit sans jamais toucher à une base de données.
type fauxStore struct {
	recu store.NewApplication
	err  error
}

func (f *fauxStore) CreateApplication(_ context.Context, a store.NewApplication) (store.Application, error) {
	f.recu = a
	if f.err != nil {
		return store.Application{}, f.err
	}
	return store.Application{ID: "id-test", Nom: a.Nom}, nil
}

func depsAvecCode(f *fauxStore) Dependencies {
	return Dependencies{
		Store:                    f,
		InvitationCode:           "code-invitation-test",
		DefaultStorageQuotaBytes: 1234,
		DefaultMaxVideos:         56,
		DefaultRequestsPerMinute: 78,
	}
}

func postApplication(t *testing.T, router http.Handler, code string, corps string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/applications", strings.NewReader(corps))
	if code != "" {
		req.Header.Set("X-Invitation-Code", code)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func codeErreur(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var rep struct {
		Erreur struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"erreur"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("réponse JSON invalide: %v (%q)", err, rec.Body.String())
	}
	if rep.Erreur.Code == "" || rep.Erreur.Message == "" {
		t.Errorf("code ou message manquant: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type inattendu: %q", ct)
	}
	return rep.Erreur.Code
}

func TestCreationSucces(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))

	rec := postApplication(t, router, "code-invitation-test", `{"nom":"  App Test  "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("en-tête Cache-Control: no-store attendu")
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
	if rep.Nom != "App Test" {
		t.Errorf("nom: attendu %q, obtenu %q", "App Test", rep.Nom)
	}
	if rep.ID != "id-test" {
		t.Errorf("id: attendu %q, obtenu %q", "id-test", rep.ID)
	}

	if f.recu.EmpreinteCle != apikey.Hash(rep.Cle) {
		t.Error("le store doit recevoir l'empreinte de la clé renvoyée")
	}
	if f.recu.EmpreinteCle == rep.Cle || strings.Contains(f.recu.EmpreinteCle, rep.Cle) {
		t.Error("le store ne doit jamais recevoir la clé en clair")
	}
	if f.recu.Nom != "App Test" {
		t.Errorf("nom transmis au store: attendu %q, obtenu %q", "App Test", f.recu.Nom)
	}
	if f.recu.PlafondStockageOctets != 1234 || f.recu.PlafondNbVideos != 56 || f.recu.LimiteRequetesParMinute != 78 {
		t.Errorf("plafonds par défaut non transmis: %+v", f.recu)
	}
}

func TestCreationSansEnTete(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "", `{"nom":"App"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_invitation_invalide" {
		t.Errorf("code d'erreur: attendu code_invitation_invalide, obtenu %q", code)
	}
	if f.recu.Nom != "" {
		t.Error("le store ne doit pas être appelé")
	}
}

func TestCreationMauvaisCode(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "mauvais-code", `{"nom":"App"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_invitation_invalide" {
		t.Errorf("code d'erreur: attendu code_invitation_invalide, obtenu %q", code)
	}
}

func TestCreationDesactivee(t *testing.T) {
	f := &fauxStore{}
	deps := depsAvecCode(f)
	deps.InvitationCode = ""
	router := NewRouter(deps)
	rec := postApplication(t, router, "n-importe-quoi", `{"nom":"App"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code HTTP: attendu 503, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "creation_desactivee" {
		t.Errorf("code d'erreur: attendu creation_desactivee, obtenu %q", code)
	}
}

func TestCreationCorpsInvalide(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "corps_invalide" {
		t.Errorf("code d'erreur: attendu corps_invalide, obtenu %q", code)
	}
}

func TestCreationNomInvalide(t *testing.T) {
	cas := []struct {
		nom   string
		corps string
	}{
		{"vide", `{"nom":"   "}`},
		{"trop long", `{"nom":"` + strings.Repeat("a", 101) + `"}`},
	}
	for _, tc := range cas {
		t.Run(tc.nom, func(t *testing.T) {
			f := &fauxStore{}
			router := NewRouter(depsAvecCode(f))
			rec := postApplication(t, router, "code-invitation-test", tc.corps)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
			}
			if code := codeErreur(t, rec); code != "nom_invalide" {
				t.Errorf("code d'erreur: attendu nom_invalide, obtenu %q", code)
			}
		})
	}
}

func TestCreationChampInconnu(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App","inconnu":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "corps_invalide" {
		t.Errorf("code d'erreur: attendu corps_invalide, obtenu %q", code)
	}
}

func TestCreationNomDejaPris(t *testing.T) {
	f := &fauxStore{err: store.ErrNomDejaUtilise}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("code HTTP: attendu 409, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "nom_deja_utilise" {
		t.Errorf("code d'erreur: attendu nom_deja_utilise, obtenu %q", code)
	}
}

func TestCreationErreurInterne(t *testing.T) {
	detail := "ERREUR_TECHNIQUE_CONFIDENTIELLE_98765"
	f := &fauxStore{err: errors.New(detail)}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code HTTP: attendu 500, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "erreur_interne" {
		t.Errorf("code d'erreur: attendu erreur_interne, obtenu %q", code)
	}
	if strings.Contains(rec.Body.String(), detail) {
		t.Error("le détail technique ne doit pas apparaître dans la réponse")
	}
}
